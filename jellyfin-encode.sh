#!/bin/bash
#===============================================================================
# Jellyfin Media Library Encoder
#===============================================================================

set -o pipefail

# Config
ENCODER_MODE="vaapi"
VAAPI_DEVICE="/dev/dri/renderD128"
VAAPI_QP=19
CPU_CRF=19
CPU_PRESET="slow"
OUTPUT_CONTAINER="mkv"
KEYFRAME_INT=48              # Keyframes every 48 frames (~2s at 24fps)

DRY_RUN=false
SKIP_EXISTING=true
SKIP_HEVC=false
LOG_FILE=""
VERBOSE=false
CHECK_ONLY=false

INPUT_DIR=""
OUTPUT_DIR=""

# Colors
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $1"
    echo -e "$msg"
    [[ -n "$LOG_FILE" ]] && echo "$msg" >> "$LOG_FILE"
    return 0
}
log_info()    { log "${BLUE}[INFO]${NC} $1"; }
log_success() { log "${GREEN}[SUCCESS]${NC} $1"; }
log_warn()    { log "${YELLOW}[WARN]${NC} $1"; }
log_error()   { log "${RED}[ERROR]${NC} $1"; }
log_debug()   { [[ "$VERBOSE" == true ]] && log "${CYAN}[DEBUG]${NC} $1"; }

usage() {
    cat << 'EOF'
Usage: jellyfin-encode.sh [OPTIONS] <input_dir> <output_dir>

Options:
  -m, --mode <vaapi|cpu>    Encoder mode (default: vaapi)
  -q, --quality <value>     QP for VAAPI, CRF for CPU (default: 19, lower=better)
  -p, --preset <preset>     CPU preset (default: slow)
  -d, --dry-run             Preview only
  --skip-hevc               HEVC files: copy video, encode audio only (fast)
  -v, --verbose             Verbose output
  -c, --check               System diagnostics
  -h, --help                Help

Encoding defaults: 10-bit HEVC, QP/CRF 19, all audio → stereo AAC 192k, keyframes every 48 frames
EOF
    exit 0
}

parse_args() {
    local positional=()
    while [[ $# -gt 0 ]]; do
        case $1 in
            -m|--mode) ENCODER_MODE="$2"; shift 2 ;;
            -q|--quality)
                [[ "$ENCODER_MODE" == "vaapi" ]] && VAAPI_QP="$2" || CPU_CRF="$2"
                shift 2 ;;
            -p|--preset) CPU_PRESET="$2"; shift 2 ;;
            -d|--dry-run) DRY_RUN=true; shift ;;
            --skip-hevc) SKIP_HEVC=true; shift ;;
            -v|--verbose) VERBOSE=true; shift ;;
            -c|--check) CHECK_ONLY=true; shift ;;
            -l|--log) LOG_FILE="$2"; shift 2 ;;
            -f|--force) SKIP_EXISTING=false; shift ;;
            -h|--help) usage ;;
            -*) log_error "Unknown: $1"; usage ;;
            *) positional+=("$1"); shift ;;
        esac
    done
    
    if [[ "$CHECK_ONLY" != true ]]; then
        [[ ${#positional[@]} -lt 2 ]] && { log_error "Need input_dir and output_dir"; usage; }
        INPUT_DIR="${positional[0]%/}"   # Strip trailing slash
        OUTPUT_DIR="${positional[1]%/}"  # Strip trailing slash
    fi
}

check_deps() {
    command -v ffmpeg &>/dev/null || { log_error "ffmpeg not found"; exit 1; }
    
    if [[ "$ENCODER_MODE" == "vaapi" ]]; then
        if [[ ! -e "$VAAPI_DEVICE" ]]; then
            VAAPI_DEVICE=$(ls /dev/dri/renderD* 2>/dev/null | head -1)
        fi
        [[ -z "$VAAPI_DEVICE" ]] && { log_error "No VAAPI device"; exit 1; }
        
        log_debug "Testing VAAPI device: $VAAPI_DEVICE"
        
        local vaapi_err
        vaapi_err=$(mktemp)
        if ! ffmpeg -hide_banner -init_hw_device vaapi=va:"$VAAPI_DEVICE" \
                -f lavfi -i color=black:s=256x256:d=0.1 \
                -vf 'format=p010,hwupload' -c:v hevc_vaapi -profile:v main10 -f null - 2>"$vaapi_err"; then
            log_error "VAAPI test failed"
            [[ "$VERBOSE" == true ]] && cat "$vaapi_err"
            rm -f "$vaapi_err"
            exit 1
        fi
        rm -f "$vaapi_err"
        log_success "VAAPI ready: $VAAPI_DEVICE (10-bit HEVC)"
    fi
}

get_codec() {
    ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of csv=p=0 "$1" 2>/dev/null | head -1
}

# Parse filename - extracts show name, season, episode
parse_filename() {
    local filename="$1"
    local parent="$2"
    local base="${filename%.*}"
    
    MEDIA_TYPE="" SHOW_NAME="" SEASON="" EPISODE="" MOVIE_NAME="" YEAR=""
    
    # SxxExx pattern
    if [[ "$base" =~ [Ss]([0-9]{1,2})[Ee]([0-9]{1,3}) ]]; then
        MEDIA_TYPE="tv"
        SEASON="${BASH_REMATCH[1]}"
        EPISODE="${BASH_REMATCH[2]}"
        # Remove SxxExx and everything after, then clean trailing dashes/spaces
        SHOW_NAME=$(echo "$base" | sed -E 's/[[:space:]._-]*[Ss][0-9]+[Ee][0-9]+.*//' | tr '._' ' ' | sed 's/[[:space:]-]*$//' | xargs)
        # Fallback to parent dir
        [[ -z "$SHOW_NAME" ]] && SHOW_NAME=$(echo "$parent" | sed -E 's/[Ss][0-9]+.*//' | tr '._' ' ' | sed 's/[[:space:]-]*$//' | xargs)
    # Anime: [Group] Name - 05
    elif [[ "$base" =~ ^(\[.+\])?[[:space:]]*(.+)[[:space:]]+-[[:space:]]*([0-9]{1,3})([[:space:]]|\[|v[0-9]|$) ]]; then
        MEDIA_TYPE="tv"
        SEASON="1"
        EPISODE="${BASH_REMATCH[3]}"
        SHOW_NAME=$(echo "${BASH_REMATCH[2]}" | xargs)
    # Anime: [Group]Name_Name_01_BD or Name_01
    elif [[ "$base" =~ ^(\[.+\])?(.+)_([0-9]{2,3})(_[^.]*)?$ ]]; then
        MEDIA_TYPE="tv"
        SEASON="1"
        EPISODE="${BASH_REMATCH[3]}"
        SHOW_NAME=$(echo "${BASH_REMATCH[2]}" | tr '_' ' ' | xargs)
    # Movie with year
    elif [[ "$base" =~ (.+)[._[:space:]]\(?([12][0-9]{3})\)? ]]; then
        MEDIA_TYPE="movie"
        MOVIE_NAME=$(echo "${BASH_REMATCH[1]}" | tr '._' ' ' | xargs)
        YEAR="${BASH_REMATCH[2]}"
    else
        MEDIA_TYPE="movie"
        MOVIE_NAME=$(echo "$base" | tr '._' ' ' | xargs)
    fi
    
    # Clean tags
    local tags='(720p|1080p|2160p|4K|WEB-DL|BluRay|x264|x265|HEVC|AAC|DTS|FLAC|10bit|HDR|Dual.Audio|EMBER|BDRip|BD|DD\+?|H\.?26[45]).*'
    SHOW_NAME=$(echo "$SHOW_NAME" | sed -E "s/$tags//i" | sed -E 's/\[[^]]*\]//g' | xargs)
    MOVIE_NAME=$(echo "$MOVIE_NAME" | sed -E "s/$tags//i" | sed -E 's/\[[^]]*\]//g' | xargs)
    
    # Title case
    SHOW_NAME=$(echo "$SHOW_NAME" | sed 's/\b\(.\)/\u\1/g')
    MOVIE_NAME=$(echo "$MOVIE_NAME" | sed 's/\b\(.\)/\u\1/g')
    
    # Fallbacks
    [[ "$MEDIA_TYPE" == "tv" && -z "$SHOW_NAME" ]] && SHOW_NAME="Unknown"
    [[ "$MEDIA_TYPE" == "movie" && -z "$MOVIE_NAME" ]] && MOVIE_NAME="Unknown"
    
    log_debug "Parsed: $MEDIA_TYPE | show='$SHOW_NAME' S${SEASON:-?}E${EPISODE:-?} | movie='$MOVIE_NAME' (${YEAR:-no year})"
}

get_output_path() {
    if [[ "$MEDIA_TYPE" == "tv" ]]; then
        local s=$(printf "%02d" "$((10#${SEASON:-1}))")
        local e=$(printf "%02d" "$((10#${EPISODE:-1}))")
        echo "$OUTPUT_DIR/$SHOW_NAME/Season $s/${SHOW_NAME} - S${s}E${e}.${OUTPUT_CONTAINER}"
    else
        local name="$MOVIE_NAME"
        [[ -n "$YEAR" ]] && name="$MOVIE_NAME ($YEAR)"
        echo "$OUTPUT_DIR/$name/${name}.${OUTPUT_CONTAINER}"
    fi
}

encode_file() {
    local input="$1" output="$2"
    
    mkdir -p "$(dirname "$output")" || { log_error "Can't create dir"; return 1; }
    
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY] $input"
        log_info "   -> $output"
        return 0
    fi
    
    log_info "Encoding: $(basename "$input")"
    log_info "  -> $(basename "$output")"
    
    local start result=0
    start=$(date +%s)
    
    # Run ffmpeg with visible progress (-stats shows frame/speed on stderr)
    if [[ "$ENCODER_MODE" == "vaapi" ]]; then
        ffmpeg -hide_banner -y -stats \
            -init_hw_device vaapi=va:"$VAAPI_DEVICE" -filter_hw_device va \
            -i "$input" -vf 'format=p010,hwupload' \
            -map 0:v:0 -map 0:a -map -0:s -map -0:t \
            -c:v hevc_vaapi -qp "$VAAPI_QP" -profile:v main10 -g "$KEYFRAME_INT" \
            -c:a aac -ac 2 -b:a 192k -map_metadata 0 \
            "$output" || result=$?
    else
        ffmpeg -hide_banner -y -stats \
            -i "$input" \
            -map 0:v:0 -map 0:a -map -0:s -map -0:t \
            -c:v libx265 -crf "$CPU_CRF" -preset "$CPU_PRESET" \
            -profile:v main10 -pix_fmt yuv420p10le -g "$KEYFRAME_INT" \
            -x265-params log-level=error \
            -c:a aac -ac 2 -b:a 192k -map_metadata 0 \
            "$output" || result=$?
    fi
    
    local elapsed=$(( $(date +%s) - start ))
    
    if [[ $result -eq 0 && -f "$output" ]]; then
        local in_sz out_sz ratio
        in_sz=$(stat -c%s "$input" 2>/dev/null) || in_sz=0
        out_sz=$(stat -c%s "$output" 2>/dev/null) || out_sz=0
        [[ $in_sz -gt 0 ]] && ratio=$((out_sz * 100 / in_sz)) || ratio=0
        log_success "Done in ${elapsed}s (${ratio}%)"
        return 0
    else
        log_error "Failed!"
        rm -f "$output"
        return 1
    fi
}

process_files() {
    local -a files=()
    local exts="mkv|mp4|avi|m4v|mov|wmv|flv|webm|ts|m2ts"
    
    while IFS= read -r -d '' f; do
        files+=("$f")
    done < <(find "$INPUT_DIR" -type f -regextype posix-extended -iregex ".*\.($exts)$" -print0 | sort -z)
    
    local total=${#files[@]} current=0 encoded=0 skipped=0 failed=0
    
    log_info "Found $total files"
    log_info "Mode: $ENCODER_MODE (10-bit HEVC), QP/CRF: $([[ $ENCODER_MODE == vaapi ]] && echo $VAAPI_QP || echo $CPU_CRF)"
    log_info "Audio: All tracks → Stereo AAC 192k | Keyframes: ${KEYFRAME_INT}f"
    [[ "$SKIP_HEVC" == true ]] && log_info "HEVC files: remux (copy video, encode audio)"
    echo
    
    for f in "${files[@]}"; do
        ((current++))
        
        # Skip files in NC/extras/sample folders
        local dirpath=$(dirname "$f")
        if [[ "$dirpath" =~ /(NC|NCOP|NCED|Extras?|Samples?|Featurettes?)(/|$) ]]; then
            log_debug "Skip (extras): $(basename "$f")"
            ((skipped++))
            continue
        fi
        
        log_info "[$current/$total] $(basename "$f")"
        
        # Parse filename first to get output path
        parse_filename "$(basename "$f")" "$(basename "$(dirname "$f")")"
        local out=$(get_output_path)
        
        # Check if already HEVC - copy video, but still encode audio to AAC
        if [[ "$SKIP_HEVC" == true ]]; then
            local codec=$(get_codec "$f")
            if [[ "$codec" == "hevc" ]]; then
                if [[ "$SKIP_EXISTING" == true && -f "$out" ]]; then
                    log_warn "Skip (exists): $(basename "$out")"
                    ((skipped++))
                    echo
                    continue
                fi
                log_info "Remuxing (HEVC→copy video, encode AAC): $(basename "$f")"
                log_info "  -> $(basename "$out")"
                mkdir -p "$(dirname "$out")"
                if [[ "$DRY_RUN" == true ]]; then
                    log_success "[DRY] Would remux"
                else
                    local start_rm=$(date +%s)
                    
                    # Copy video, encode all audio to stereo AAC, exclude subs & attachments
                    ffmpeg -hide_banner -y -stats \
                        -i "$f" \
                        -map 0:v:0 -map 0:a -map -0:s -map -0:t \
                        -c:v copy \
                        -c:a aac -ac 2 -b:a 192k \
                        -map_metadata 0 \
                        "$out" || { log_error "Remux failed"; ((failed++)); echo; continue; }
                    
                    local elapsed_rm=$(( $(date +%s) - start_rm ))
                    local in_sz=$(stat -c%s "$f" 2>/dev/null) || in_sz=0
                    local out_sz=$(stat -c%s "$out" 2>/dev/null) || out_sz=0
                    [[ $in_sz -gt 0 ]] && ratio=$((out_sz * 100 / in_sz)) || ratio=100
                    log_success "Remuxed in ${elapsed_rm}s (${ratio}% of original)"
                fi
                ((encoded++))
                echo
                continue
            fi
        fi
        
        if [[ "$SKIP_EXISTING" == true && -f "$out" ]]; then
            log_warn "Skip (exists)"
            ((skipped++))
            echo
            continue
        fi
        
        if encode_file "$f" "$out"; then
            ((encoded++))
        else
            ((failed++))
        fi
        echo
    done
    
    log_info "=============================="
    log_info "Done: $encoded encoded, $skipped skipped, $failed failed"
}

run_check() {
    log_info "=== System Check ==="
    
    if command -v ffmpeg &>/dev/null; then
        log_success "ffmpeg: $(ffmpeg -version 2>/dev/null | head -1)"
    else
        log_error "ffmpeg not found"
    fi
    
    echo "HEVC encoders:"
    ffmpeg -hide_banner -encoders 2>/dev/null | grep -E "hevc|265"
    
    local dev=$(ls /dev/dri/renderD* 2>/dev/null | head -1)
    if [[ -n "$dev" ]]; then
        log_info "Testing VAAPI on $dev..."
        if ffmpeg -hide_banner -init_hw_device vaapi=va:"$dev" \
                -f lavfi -i color=black:s=256x256:d=0.1 \
                -vf 'format=p010,hwupload' -c:v hevc_vaapi -profile:v main10 -f null - 2>/dev/null; then
            log_success "VAAPI works"
        else
            log_error "VAAPI failed"
        fi
    fi
    
    log_info "Testing CPU x265..."
    if ffmpeg -hide_banner -f lavfi -i color=black:s=256x256:d=0.1 -c:v libx265 -f null - 2>/dev/null; then
        log_success "CPU x265 works"
    else
        log_error "CPU x265 failed"
    fi
}

main() {
    parse_args "$@"
    
    if [[ "$CHECK_ONLY" == true ]]; then
        run_check
        exit 0
    fi
    
    [[ ! -d "$INPUT_DIR" ]] && { log_error "Input not found: $INPUT_DIR"; exit 1; }
    mkdir -p "$OUTPUT_DIR"
    
    log_info "=== Jellyfin Encoder ==="
    log_info "In:  $INPUT_DIR"
    log_info "Out: $OUTPUT_DIR"
    [[ "$DRY_RUN" == true ]] && log_warn "DRY RUN"
    echo
    
    check_deps
    process_files
}

main "$@"

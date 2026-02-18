#!/bin/bash
#===============================================================================
# Muxmaster Media Library Encoder v1.4.0
# Comprehensive HEVC/AAC encoding for Jellyfin optimization
#===============================================================================

set -o pipefail

#------------------------------------------------------------------------------
# Configuration defaults
#------------------------------------------------------------------------------
# Encoding defaults
ENCODER_MODE="vaapi"
VAAPI_DEVICE="/dev/dri/renderD128"
VAAPI_QP=19
VAAPI_PROFILE="main10"
VAAPI_SW_FORMAT="p010"
CPU_CRF=19
CPU_PRESET="slow"
CPU_HEVC_PROFILE="main10"
CPU_PIX_FMT="yuv420p10le"
OUTPUT_CONTAINER="mkv"
KEYFRAME_INT=48
AUDIO_CHANNELS=2
AUDIO_BITRATE="224k"
FFMPEG_PROBESIZE="100M"
FFMPEG_ANALYZEDURATION="100M"

# Logging/UX defaults
FFMPEG_LOGLEVEL="error"
declare -a FFMPEG_PROGRESS_ARGS=()

# Stream retention defaults
KEEP_SUBTITLES=true
KEEP_ATTACHMENTS=true

# Runtime behavior defaults
DRY_RUN=false
SKIP_EXISTING=true
SKIP_HEVC=true
SHOW_FILE_STATS=true
SHOW_FFMPEG_FPS=true
LOG_FILE=""
VERBOSE=false
CHECK_ONLY=false
QUALITY_OVERRIDE=""
COLOR_MODE="auto"
STRICT_MODE=false
CLEAN_TIMESTAMPS=true
MATCH_AUDIO_LAYOUT=true
HANDLE_HDR="preserve"
DEINTERLACE_AUTO=true

INPUT_DIR=""
OUTPUT_DIR=""
SCRIPT_NAME="$(basename "$0")"
SCRIPT_VERSION="1.4.0"

# Temp file tracking for cleanup
declare -a TEMP_FILES=()
# Per-file CSV summary rows
declare -a FILE_SUMMARY_ROWS=()

# ANSI color palette
RED=""; GREEN=""; YELLOW=""; BLUE=""; CYAN=""; NC=""

#------------------------------------------------------------------------------
# Cleanup trap - ensures temp files are removed on exit/interrupt
#------------------------------------------------------------------------------
cleanup_temp_files() {
    local f
    for f in "${TEMP_FILES[@]}"; do
        [[ -f "$f" ]] && rm -f "$f"
    done
}
trap cleanup_temp_files EXIT INT TERM

# Create tracked temp file
make_temp_file() {
    local tmp
    tmp=$(mktemp)
    TEMP_FILES+=("$tmp")
    printf '%s\n' "$tmp"
}

#------------------------------------------------------------------------------
# Color initialization
#------------------------------------------------------------------------------
init_colors() {
    local enable_colors=false

    case "$COLOR_MODE" in
        always) enable_colors=true ;;
        never)  enable_colors=false ;;
        auto)
            if [[ -t 1 && -z "${NO_COLOR:-}" && "${TERM:-}" != "dumb" ]]; then
                enable_colors=true
            fi
            ;;
    esac

    if [[ "$enable_colors" == true ]]; then
        RED=$'\033[1;91m'
        GREEN=$'\033[1;92m'
        YELLOW=$'\033[1;93m'
        BLUE=$'\033[1;94m'
        CYAN=$'\033[1;96m'
        NC=$'\033[0m'
    else
        RED=""; GREEN=""; YELLOW=""; BLUE=""; CYAN=""; NC=""
    fi
}

#------------------------------------------------------------------------------
# Banner
#------------------------------------------------------------------------------
print_banner() {
    if [[ -n "$CYAN" ]]; then
        printf '%b' $'\033[1;95m'
    fi

    cat << 'EOF'
 __  __            __  __           _
|  \/  |_   ___  _|  \/  | __ _ ___| |_ ___ _ __
| |\/| | | | \ \/ / |\/| |/ _` / __| __/ _ \ '__|
| |  | | |_| |>  <| |  | | (_| \__ \ ||  __/ |
|_|  |_|\__,_/_/\_\_|  |_|\__,_|___/\__\___|_|
EOF

    if [[ -n "$CYAN" ]]; then
        printf '%b\n' "$NC"
    fi
}

#------------------------------------------------------------------------------
# Logging functions
#------------------------------------------------------------------------------
log_line() {
    local level="$1"
    local color="$2"
    local text="$3"
    local ts="[$(date '+%Y-%m-%d %H:%M:%S')]"
    local stream_fd=1

    [[ "$level" == "ERROR" ]] && stream_fd=2

    if [[ -n "$color" ]]; then
        printf '%s %b[%s]%b %s\n' "$ts" "$color" "$level" "$NC" "$text" >&"$stream_fd"
    else
        printf '%s [%s] %s\n' "$ts" "$level" "$text" >&"$stream_fd"
    fi

    if [[ -n "$LOG_FILE" ]]; then
        printf '%s [%s] %s\n' "$ts" "$level" "$text" >> "$LOG_FILE"
    fi

    return 0
}
log_info()    { log_line "INFO" "$BLUE" "$1"; }
log_success() { log_line "SUCCESS" "$GREEN" "$1"; }
log_warn()    { log_line "WARN" "$YELLOW" "$1"; }
log_error()   { log_line "ERROR" "$RED" "$1"; }
log_debug()   { [[ "$VERBOSE" == true ]] && log_line "DEBUG" "$CYAN" "$1"; return 0; }

#------------------------------------------------------------------------------
# CLI help
#------------------------------------------------------------------------------
usage() {
    local exit_code="${1:-0}"
    local usage_stream=1
    [[ "$exit_code" -ne 0 ]] && usage_stream=2

    cat >&"$usage_stream" << EOF
Muxmaster v$SCRIPT_VERSION - Jellyfin-Optimized Media Encoder

Usage: $SCRIPT_NAME [OPTIONS] <input_dir> <output_dir>

Encoding Options:
  -m, --mode <vaapi|cpu>    Encoder mode (default: vaapi)
  -q, --quality <value>     QP for VAAPI, CRF for CPU (defaults: VAAPI=19, CPU=19)
  -p, --preset <preset>     CPU preset (default: slow)

HDR/Color Options:
  --hdr <preserve|tonemap>  HDR handling (default: preserve)
  --no-deinterlace          Disable automatic deinterlace detection

Stream Options:
  --skip-hevc               Copy HEVC video, encode audio only (default: on)
  --no-skip-hevc            Re-encode HEVC video
  --no-subs                 Do not process subtitle streams
  --no-attachments          Do not include attachments (fonts/images)

Output Options:
  --container <mkv|mp4>     Output container (default: mkv)
  -f, --force               Overwrite existing output files

Behavior Options:
  -d, --dry-run             Preview only
  --strict                  Disable automatic ffmpeg retry fallbacks
  --clean-timestamps        Enable timestamp regeneration (default: on)
  --no-clean-timestamps     Disable timestamp regeneration
  --match-audio-layout      Normalize encoded audio layout (default: on)
  --no-match-audio-layout   Disable audio layout normalization

Display Options:
  --show-fps                Show live ffmpeg FPS/speed (default: on)
  --no-fps                  Disable live FPS progress
  --no-stats                Hide per-file source stats
  --color                   Force colored logs
  --no-color                Disable colored logs
  -v, --verbose             Verbose output

Utility:
  -l, --log <path>          Write logs to file
  -c, --check               System diagnostics
  -V, --version             Print version
  -h, --help                Help

Encoding: 10-bit HEVC, copy AAC or encode non-AAC to AAC, MKV container, metadata preserved
EOF
    exit "$exit_code"
}

show_version() {
    printf '%s v%s\n' "$SCRIPT_NAME" "$SCRIPT_VERSION"
}

require_option_value() {
    local opt="$1"
    local value="${2-}"
    if [[ -z "$value" ]]; then
        log_error "Option '$opt' requires a value"
        usage 1
    fi
}

normalize_dir_arg() {
    local path="$1"
    if [[ "$path" == "/" ]]; then
        printf '/\n'
        return 0
    fi

    while [[ "$path" == */ ]]; do
        path="${path%/}"
    done

    printf '%s\n' "$path"
}

#------------------------------------------------------------------------------
# Argument parsing
#------------------------------------------------------------------------------
parse_args() {
    local positional=()
    while [[ $# -gt 0 ]]; do
        case $1 in
            --)
                shift
                positional+=("$@")
                break
                ;;
            -m|--mode)
                require_option_value "$1" "${2-}"
                ENCODER_MODE="$2"
                shift 2
                ;;
            -q|--quality)
                require_option_value "$1" "${2-}"
                QUALITY_OVERRIDE="$2"
                shift 2
                ;;
            -p|--preset)
                require_option_value "$1" "${2-}"
                CPU_PRESET="$2"
                shift 2
                ;;
            --container)
                require_option_value "$1" "${2-}"
                OUTPUT_CONTAINER="${2,,}"
                shift 2
                ;;
            --hdr)
                require_option_value "$1" "${2-}"
                HANDLE_HDR="${2,,}"
                shift 2
                ;;
            --no-deinterlace)
                DEINTERLACE_AUTO=false
                shift
                ;;
            -d|--dry-run) DRY_RUN=true; shift ;;
            --skip-hevc) SKIP_HEVC=true; shift ;;
            --no-skip-hevc) SKIP_HEVC=false; shift ;;
            --show-fps) SHOW_FFMPEG_FPS=true; shift ;;
            --no-fps) SHOW_FFMPEG_FPS=false; shift ;;
            --no-stats) SHOW_FILE_STATS=false; shift ;;
            --no-subs) KEEP_SUBTITLES=false; shift ;;
            --no-attachments) KEEP_ATTACHMENTS=false; shift ;;
            --strict) STRICT_MODE=true; shift ;;
            --clean-timestamps) CLEAN_TIMESTAMPS=true; shift ;;
            --no-clean-timestamps) CLEAN_TIMESTAMPS=false; shift ;;
            --match-audio-layout) MATCH_AUDIO_LAYOUT=true; shift ;;
            --no-match-audio-layout) MATCH_AUDIO_LAYOUT=false; shift ;;
            --color) COLOR_MODE="always"; shift ;;
            --no-color) COLOR_MODE="never"; shift ;;
            -v|--verbose) VERBOSE=true; shift ;;
            -c|--check) CHECK_ONLY=true; shift ;;
            -V|--version) show_version; exit 0 ;;
            -l|--log)
                require_option_value "$1" "${2-}"
                LOG_FILE="$2"
                shift 2
                ;;
            -f|--force) SKIP_EXISTING=false; shift ;;
            -h|--help) usage 0 ;;
            -*) log_error "Unknown option: $1"; usage 1 ;;
            *) positional+=("$1"); shift ;;
        esac
    done

    if [[ "$CHECK_ONLY" != true ]]; then
        [[ ${#positional[@]} -ne 2 ]] && { log_error "Need exactly input_dir and output_dir"; usage 1; }
        INPUT_DIR="$(normalize_dir_arg "${positional[0]}")"
        OUTPUT_DIR="$(normalize_dir_arg "${positional[1]}")"
    fi

    case "$ENCODER_MODE" in
        vaapi|cpu) ;;
        *)
            log_error "Invalid mode '$ENCODER_MODE' (use 'vaapi' or 'cpu')"
            exit 1
            ;;
    esac

    case "$OUTPUT_CONTAINER" in
        mkv|mp4) ;;
        *)
            log_error "Invalid container '$OUTPUT_CONTAINER' (use 'mkv' or 'mp4')"
            exit 1
            ;;
    esac

    case "$HANDLE_HDR" in
        preserve|tonemap) ;;
        *)
            log_error "Invalid HDR mode '$HANDLE_HDR' (use 'preserve' or 'tonemap')"
            exit 1
            ;;
    esac

    if [[ -n "$QUALITY_OVERRIDE" ]]; then
        if ! [[ "$QUALITY_OVERRIDE" =~ ^[0-9]+$ ]]; then
            log_error "Quality must be a whole number (got '$QUALITY_OVERRIDE')"
            exit 1
        fi

        if [[ "$ENCODER_MODE" == "vaapi" ]]; then
            VAAPI_QP="$QUALITY_OVERRIDE"
        else
            CPU_CRF="$QUALITY_OVERRIDE"
        fi
    fi

    if [[ "$VERBOSE" == true ]]; then
        FFMPEG_LOGLEVEL="info"
    else
        FFMPEG_LOGLEVEL="error"
    fi

    if [[ "$VERBOSE" == true || "$SHOW_FFMPEG_FPS" == true ]]; then
        FFMPEG_PROGRESS_ARGS=(-stats -stats_period 1)
    else
        FFMPEG_PROGRESS_ARGS=()
    fi
}

#------------------------------------------------------------------------------
# Portable file size function (Linux + macOS)
#------------------------------------------------------------------------------
get_file_size() {
    local file="$1"
    if [[ "$(uname)" == "Darwin" ]]; then
        stat -f%z "$file" 2>/dev/null || echo 0
    else
        stat -c%s "$file" 2>/dev/null || echo 0
    fi
}

#------------------------------------------------------------------------------
# CSV summary helpers
#------------------------------------------------------------------------------
csv_escape_field() {
    local value="${1-}"
    value=${value//\"/\"\"}
    printf '"%s"' "$value"
}

append_file_csv_summary() {
    local input_file="$1" output_file="$2" media_type="$3" action="$4" video_action="$5" audio_action="$6" status="$7" note="$8"
    local row

    row="$(csv_escape_field "$input_file"),$(csv_escape_field "$output_file"),$(csv_escape_field "$media_type"),$(csv_escape_field "$action"),$(csv_escape_field "$video_action"),$(csv_escape_field "$audio_action"),$(csv_escape_field "$status"),$(csv_escape_field "$note")"
    FILE_SUMMARY_ROWS+=("$row")
}

print_file_csv_summary() {
    local header="input_file,output_file,media_type,action,video_action,audio_action,status,note"
    local row

    echo
    log_info "Per-file summary (CSV)"
    printf '%s\n' "$header"
    for row in "${FILE_SUMMARY_ROWS[@]}"; do
        printf '%s\n' "$row"
    done

    if [[ -n "$LOG_FILE" ]]; then
        printf '%s\n' "$header" >> "$LOG_FILE"
        for row in "${FILE_SUMMARY_ROWS[@]}"; do
            printf '%s\n' "$row" >> "$LOG_FILE"
        done
    fi
}

#------------------------------------------------------------------------------
# VAAPI device and encoder tests
#------------------------------------------------------------------------------
test_vaapi_profile() {
    local sw_format="$1"
    local profile="$2"
    local err_file="$3"

    ffmpeg -hide_banner -nostdin -loglevel error \
        -init_hw_device vaapi=va:"$VAAPI_DEVICE" -filter_hw_device va \
        -f lavfi -i color=black:s=256x256:d=0.1 \
        -vf "format=${sw_format},hwupload" \
        -c:v hevc_vaapi -profile:v "$profile" -f null - > /dev/null 2>"$err_file"
}

get_first_render_device() {
    local dev
    for dev in /dev/dri/renderD*; do
        [[ -e "$dev" ]] || continue
        printf '%s\n' "$dev"
        return 0
    done
    return 1
}

output_container_is_mp4() {
    [[ "${OUTPUT_CONTAINER,,}" == "mp4" ]]
}

#------------------------------------------------------------------------------
# Dependency and encoder validation
#------------------------------------------------------------------------------
check_deps() {
    command -v ffmpeg &>/dev/null || { log_error "ffmpeg not found"; exit 1; }
    command -v ffprobe &>/dev/null || { log_error "ffprobe not found"; exit 1; }

    if [[ "$ENCODER_MODE" == "vaapi" ]]; then
        if [[ ! -e "$VAAPI_DEVICE" ]]; then
            VAAPI_DEVICE=$(get_first_render_device || true)
        fi
        [[ -z "$VAAPI_DEVICE" || ! -e "$VAAPI_DEVICE" ]] && { log_error "No VAAPI device"; exit 1; }

        log_debug "Testing VAAPI device: $VAAPI_DEVICE"

        local vaapi_err
        vaapi_err=$(make_temp_file)
        if test_vaapi_profile "p010" "main10" "$vaapi_err"; then
            VAAPI_SW_FORMAT="p010"
            VAAPI_PROFILE="main10"
            log_success "VAAPI ready: $VAAPI_DEVICE (HEVC main10)"
        elif test_vaapi_profile "nv12" "main" "$vaapi_err"; then
            VAAPI_SW_FORMAT="nv12"
            VAAPI_PROFILE="main"
            log_warn "VAAPI main10 unavailable, falling back to HEVC main (8-bit)"
            log_success "VAAPI ready: $VAAPI_DEVICE (HEVC main)"
        else
            log_error "VAAPI test failed"
            [[ "$VERBOSE" == true ]] && sed 's/^/  /' "$vaapi_err"
            exit 1
        fi
    else
        if ! ffmpeg -hide_banner -nostdin -loglevel error \
                -f lavfi -i color=black:s=256x256:d=0.1 \
                -c:v libx265 -f null - > /dev/null 2>&1; then
            log_error "CPU mode selected but libx265 is unavailable"
            exit 1
        fi
    fi
}

#------------------------------------------------------------------------------
# Stream analysis functions
#------------------------------------------------------------------------------
get_codec() {
    ffprobe -v error -select_streams v:0 -show_entries stream=codec_name \
        -of default=noprint_wrappers=1:nokey=1 "$1" 2>/dev/null | head -1
}

has_audio_stream() {
    local count
    count=$(get_stream_count "a" "$1")
    [[ "$count" -gt 0 ]]
}

get_stream_count() {
    local selector="$1"
    local input="$2"
    local count

    count=$(ffprobe -v error -select_streams "$selector" -show_entries stream=index \
        -of csv=p=0 "$input" 2>/dev/null | sed '/^[[:space:]]*$/d' | wc -l | tr -d '[:space:]')
    [[ "$count" =~ ^[0-9]+$ ]] || count=0
    printf '%s\n' "$count"
}

# Get primary video stream index (skip cover art/attached pics)
get_primary_video_stream_index() {
    local idx codec attached
    while IFS='|' read -r idx codec attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" != "1" ]] && { echo "$idx"; return 0; }
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,codec_name:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    # Return empty string if no valid video stream found
    echo ""
}

get_primary_video_codec() {
    local idx codec attached
    while IFS='|' read -r idx codec attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" != "1" ]] && { echo "$codec"; return 0; }
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,codec_name:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    get_codec "$1"
}

get_primary_video_profile() {
    local idx profile attached
    while IFS='|' read -r idx profile attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" != "1" ]] && { echo "$profile"; return 0; }
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,profile:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    echo ""
}

get_primary_video_pix_fmt() {
    local idx pix_fmt attached
    while IFS='|' read -r idx pix_fmt attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" != "1" ]] && { echo "$pix_fmt"; return 0; }
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,pix_fmt:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    echo ""
}

get_primary_video_resolution() {
    local idx width height attached
    while IFS='|' read -r idx width height attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" == "1" ]] && continue

        if [[ "$width" =~ ^[0-9]+$ && "$height" =~ ^[0-9]+$ && "$width" -gt 0 && "$height" -gt 0 ]]; then
            printf '%sx%s\n' "$width" "$height"
        else
            printf 'unknown\n'
        fi
        return 0
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,width,height:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    printf 'unknown\n'
}

get_primary_video_bitrate_bps() {
    local idx bit_rate attached
    while IFS='|' read -r idx bit_rate attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" == "1" ]] && continue
        [[ "$bit_rate" =~ ^[0-9]+$ && "$bit_rate" -gt 0 ]] && { echo "$bit_rate"; return 0; }
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,bit_rate:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    local format_bitrate
    format_bitrate=$(ffprobe -v error -show_entries format=bit_rate \
        -of default=noprint_wrappers=1:nokey=1 "$1" 2>/dev/null | head -1)
    [[ "$format_bitrate" =~ ^[0-9]+$ && "$format_bitrate" -gt 0 ]] && { echo "$format_bitrate"; return 0; }

    echo ""
}

format_bitrate_label() {
    local bitrate_bps="$1"

    if [[ "$bitrate_bps" =~ ^[0-9]+$ && "$bitrate_bps" -gt 0 ]]; then
        printf '%d kb/s\n' "$(((bitrate_bps + 500) / 1000))"
    else
        printf 'unknown\n'
    fi
}

get_primary_video_bitrate_label() {
    local bitrate_bps
    bitrate_bps=$(get_primary_video_bitrate_bps "$1")
    format_bitrate_label "$bitrate_bps"
}

#------------------------------------------------------------------------------
# HDR and color space detection
#------------------------------------------------------------------------------
get_primary_video_color_fields() {
    local input="$1"
    local idx color_transfer color_primaries color_space attached

    while IFS='|' read -r idx color_transfer color_primaries color_space attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" == "1" ]] && continue
        printf '%s:%s:%s\n' "${color_transfer:-unknown}" "${color_primaries:-unknown}" "${color_space:-unknown}"
        return 0
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,color_transfer,color_primaries,color_space:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$input" 2>/dev/null)

    printf 'unknown:unknown:unknown\n'
}

detect_hdr_type() {
    local input="$1"
    local color_transfer color_primaries color_space

    IFS=':' read -r color_transfer color_primaries color_space <<< "$(get_primary_video_color_fields "$input")"

    # Check for HDR indicators
    case "$color_transfer" in
        smpte2084|arib-std-b67)
            echo "hdr10"
            return 0
            ;;
    esac

    case "$color_primaries" in
        bt2020)
            echo "hdr10"
            return 0
            ;;
    esac

    echo "sdr"
}

get_color_metadata() {
    local input="$1"
    get_primary_video_color_fields "$input"
}

#------------------------------------------------------------------------------
# Interlace detection
#------------------------------------------------------------------------------
is_interlaced() {
    local input="$1"
    local idx field_order attached

    while IFS='|' read -r idx field_order attached; do
        [[ -z "$idx" ]] && continue
        [[ "$attached" == "1" ]] && continue
        case "$field_order" in
            tt|bb|tb|bt)
                return 0
                ;;
        esac
        return 1
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,field_order:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$input" 2>/dev/null)

    return 1
}

#------------------------------------------------------------------------------
# Audio channel detection
#------------------------------------------------------------------------------
get_audio_channels() {
    local input="$1"
    local stream_idx="${2:-0}"
    local channels

    channels=$(ffprobe -v error -select_streams "a:$stream_idx" \
        -show_entries stream=channels \
        -of default=noprint_wrappers=1:nokey=1 "$input" 2>/dev/null | head -1)

    [[ "$channels" =~ ^[0-9]+$ ]] || channels=2
    printf '%s\n' "$channels"
}

get_audio_codec() {
    local input="$1"
    local stream_idx="${2:-0}"
    local codec

    codec=$(ffprobe -v error -select_streams "a:$stream_idx" \
        -show_entries stream=codec_name \
        -of default=noprint_wrappers=1:nokey=1 "$input" 2>/dev/null | head -1)

    printf '%s\n' "$codec"
}

#------------------------------------------------------------------------------
# Subtitle format detection
#------------------------------------------------------------------------------
get_subtitle_codecs() {
    local input="$1"
    ffprobe -v error -select_streams s \
        -show_entries stream=codec_name \
        -of csv=p=0 "$input" 2>/dev/null
}

has_bitmap_subtitles() {
    local input="$1"
    local codec
    while read -r codec; do
        case "$codec" in
            hdmv_pgs_subtitle|dvd_subtitle|dvb_subtitle|xsub)
                return 0
                ;;
        esac
    done < <(get_subtitle_codecs "$input")
    return 1
}

#------------------------------------------------------------------------------
# HEVC stream safety check for browser/Jellyfin compatibility
#------------------------------------------------------------------------------
is_edge_safe_hevc_stream() {
    local profile_lower pix_fmt_lower
    profile_lower=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
    pix_fmt_lower=$(printf '%s' "$2" | tr '[:upper:]' '[:lower:]')

    case "$profile_lower" in
        main|main\ 10|main10) ;;
        *) return 1 ;;
    esac

    case "$pix_fmt_lower" in
        yuv420p|yuv420p10le) return 0 ;;
        *) return 1 ;;
    esac
}

#------------------------------------------------------------------------------
# File validation
#------------------------------------------------------------------------------
validate_input_file() {
    local input="$1"

    # Check file exists and is readable
    [[ ! -f "$input" ]] && { log_error "File not found: $input"; return 1; }
    [[ ! -r "$input" ]] && { log_error "File not readable: $input"; return 1; }

    # Check file has content
    local size
    size=$(get_file_size "$input")
    [[ "$size" -lt 1000 ]] && { log_error "File too small (possibly corrupt): $input"; return 1; }

    # Check file is a valid media container
    if ! ffprobe -v error -show_entries format=duration "$input" &>/dev/null; then
        log_error "Cannot probe file (possibly corrupt): $input"
        return 1
    fi

    return 0
}

#------------------------------------------------------------------------------
# FFmpeg execution helpers
#------------------------------------------------------------------------------
run_ffmpeg_logged() {
    local err_file="$1"
    shift

    if [[ "$VERBOSE" == true || "$SHOW_FFMPEG_FPS" == true ]]; then
        "$@" 2> >(tee "$err_file" >&2)
    else
        "$@" 2>"$err_file"
    fi
}

#------------------------------------------------------------------------------
# Error pattern matchers for retry logic
#------------------------------------------------------------------------------
ffmpeg_error_has_attachment_tag_issue() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eq 'Attachment stream [0-9]+ has no (filename|mimetype) tag' "$err_file"
}

ffmpeg_error_has_subtitle_mux_issue() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eqi 'Subtitle codec .* is not supported|Could not find tag for codec .* in stream .*subtitle|Error initializing output stream .*subtitle|Error while opening encoder for output stream .*subtitle|Subtitle encoding currently only possible from text to text or bitmap to bitmap|Unknown encoder|Codec .* is not supported' "$err_file"
}

ffmpeg_error_has_mux_queue_overflow() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eq 'Too many packets buffered for output stream' "$err_file"
}

ffmpeg_error_has_timestamp_discontinuity() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eqi 'Non-monotonous DTS|non monotonically increasing dts|invalid, non monotonically increasing dts|DTS .*out of order|PTS .*out of order|pts has no value|missing PTS|Timestamps are unset' "$err_file"
}

#------------------------------------------------------------------------------
# Build video filter chain
#------------------------------------------------------------------------------
build_video_filter() {
    local input="$1"
    local encoder_mode="$2"
    local -a filters=()

    # Deinterlace if needed
    if [[ "$DEINTERLACE_AUTO" == true ]] && is_interlaced "$input"; then
        log_info "  Detected interlaced content, applying yadif deinterlacer" >&2
        filters+=("yadif=mode=send_frame:parity=auto:deint=interlaced")
    fi

    # HDR handling
    local hdr_type
    hdr_type=$(detect_hdr_type "$input")

    if [[ "$hdr_type" == "hdr10" && "$HANDLE_HDR" == "tonemap" ]]; then
        log_info "  HDR detected, applying tonemapping to SDR" >&2
        filters+=("zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p")
    fi

    if [[ "$encoder_mode" == "vaapi" ]]; then
        # VAAPI needs hwupload
        filters+=("format=${VAAPI_SW_FORMAT},hwupload")
    fi

    # Join filters with comma
    local IFS=','
    echo "${filters[*]}"
}

#------------------------------------------------------------------------------
# Build audio encoding options
#------------------------------------------------------------------------------

# Check if all audio streams are already AAC
all_audio_is_compatible_aac() {
    local input="$1"
    local stream_count codec i

    stream_count=$(get_stream_count "a" "$input")
    [[ "$stream_count" -eq 0 ]] && return 1

    for ((i=0; i<stream_count; i++)); do
        codec=$(get_audio_codec "$input" "$i")
        [[ "$codec" != "aac" ]] && return 1
    done

    return 0
}

build_audio_opts() {
    local input="$1"
    local -a opts=()

    if ! has_audio_stream "$input"; then
        opts=(-an)
    elif all_audio_is_compatible_aac "$input"; then
        # Source audio is already AAC — copy to avoid lossy AAC->AAC re-encode
        log_debug "All audio tracks are AAC, copying" >&2
        opts=(-map 0:a -c:a copy)
    else
        # Build per-stream audio strategy:
        # - copy AAC streams as-is (never AAC->AAC re-encode)
        # - encode remaining streams to AAC
        local stream_count i source_channels target_channels source_codec layout
        stream_count=$(get_stream_count "a" "$input")

        for ((i=0; i<stream_count; i++)); do
            source_codec=$(get_audio_codec "$input" "$i")
            source_channels=$(get_audio_channels "$input" "$i")

            target_channels="$source_channels"
            [[ "$target_channels" -lt 1 ]] && target_channels=1
            [[ "$target_channels" -gt "$AUDIO_CHANNELS" ]] && target_channels="$AUDIO_CHANNELS"

            opts+=(-map "0:a:$i")

            if [[ "$source_codec" == "aac" ]]; then
                log_debug "Audio stream a:$i is AAC, copying" >&2
                opts+=(-c:a:"$i" copy)
                continue
            fi

            opts+=(-c:a:"$i" aac -ac:a:"$i" "$target_channels" -ar:a:"$i" 48000 -b:a:"$i" "$AUDIO_BITRATE")

            if [[ "$MATCH_AUDIO_LAYOUT" == true ]]; then
                case "$target_channels" in
                    1) layout="mono" ;;
                    2) layout="stereo" ;;
                    *) layout="" ;;
                esac

                if [[ -n "$layout" ]]; then
                    opts+=(-filter:a:"$i" "aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=48000:channel_layouts=${layout}")
                else
                    opts+=(-filter:a:"$i" "aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=48000")
                fi
            fi
        done
    fi

    printf '%s\n' "${opts[*]}"
}

#------------------------------------------------------------------------------
# Pre-flight render plan summaries
#------------------------------------------------------------------------------
describe_audio_plan() {
    local input="$1"
    local stream_count codec i copy_count=0 transcode_count=0

    stream_count=$(get_stream_count "a" "$input")
    if [[ "$stream_count" -eq 0 ]]; then
        printf 'transcode=n/a (no audio streams)'
        return 0
    fi

    for ((i=0; i<stream_count; i++)); do
        codec=$(get_audio_codec "$input" "$i")
        if [[ "$codec" == "aac" ]]; then
            ((copy_count++))
        else
            ((transcode_count++))
        fi
    done

    if [[ "$transcode_count" -eq 0 ]]; then
        printf 'transcode=no (copy all %d AAC stream(s))' "$copy_count"
    elif [[ "$copy_count" -eq 0 ]]; then
        printf 'transcode=yes (all %d stream(s) -> AAC %s, 48kHz, up to %dch)' "$stream_count" "$AUDIO_BITRATE" "$AUDIO_CHANNELS"
    else
        printf 'transcode=yes (copy %d AAC stream(s), transcode %d stream(s) -> AAC %s, 48kHz, up to %dch)' "$copy_count" "$transcode_count" "$AUDIO_BITRATE" "$AUDIO_CHANNELS"
    fi
}

describe_subtitle_plan() {
    local input="$1"
    local include_subtitles="$2"

    if [[ "$KEEP_SUBTITLES" != true || "$include_subtitles" != true ]]; then
        printf 'disabled'
        return 0
    fi

    if output_container_is_mp4; then
        if has_bitmap_subtitles "$input"; then
            printf 'skip bitmap subtitles (MP4 incompatible)'
        else
            printf 'transcode text subtitles to mov_text'
        fi
    else
        printf 'copy subtitle streams'
    fi
}

describe_attachment_plan() {
    local include_attachments="$1"

    if [[ "$KEEP_ATTACHMENTS" != true || "$include_attachments" != true ]]; then
        printf 'disabled'
        return 0
    fi

    if output_container_is_mp4; then
        printf 'disabled for MP4'
    else
        printf 'copy attachments'
    fi
}

log_encode_render_plan() {
    local input="$1" video_stream_idx="$2" include_subtitles="$3" include_attachments="$4" muxing_queue_size="$5" timestamp_fix="$6"
    local source_video_codec source_codec_label
    local audio_plan subtitle_plan attachment_plan container_plan hdr_plan deinterlace_plan video_plan hdr_type

    source_video_codec=$(get_primary_video_codec "$input")
    [[ -z "$source_video_codec" ]] && source_video_codec=$(get_codec "$input")
    source_codec_label="${source_video_codec:-unknown}"

    if [[ "$ENCODER_MODE" == "vaapi" ]]; then
        video_plan="transcode=yes (stream ${video_stream_idx}: ${source_codec_label} -> hevc_vaapi, profile=${VAAPI_PROFILE}, QP=${VAAPI_QP}, keyint=${KEYFRAME_INT})"
    else
        video_plan="transcode=yes (stream ${video_stream_idx}: ${source_codec_label} -> libx265, CRF=${CPU_CRF}, preset=${CPU_PRESET}, profile=${CPU_HEVC_PROFILE}, pix_fmt=${CPU_PIX_FMT}, keyint=${KEYFRAME_INT})"
    fi

    audio_plan=$(describe_audio_plan "$input")
    subtitle_plan=$(describe_subtitle_plan "$input" "$include_subtitles")
    attachment_plan=$(describe_attachment_plan "$include_attachments")

    if output_container_is_mp4; then
        container_plan="MP4 (faststart + hvc1 tag)"
    else
        container_plan="${OUTPUT_CONTAINER^^}"
    fi

    hdr_type=$(detect_hdr_type "$input")
    if [[ "$hdr_type" == "hdr10" && "$HANDLE_HDR" == "tonemap" ]]; then
        hdr_plan="tonemap HDR -> SDR"
    elif [[ "$hdr_type" == "hdr10" && "$HANDLE_HDR" == "preserve" ]]; then
        hdr_plan="preserve HDR metadata"
    else
        hdr_plan="no HDR transform"
    fi

    if [[ "$DEINTERLACE_AUTO" == true ]] && is_interlaced "$input"; then
        deinterlace_plan="auto=yadif (interlaced source detected)"
    elif [[ "$DEINTERLACE_AUTO" == true ]]; then
        deinterlace_plan="auto=enabled (progressive source, no yadif)"
    else
        deinterlace_plan="disabled"
    fi

    log_info "Pre-flight render params:"
    log_info "  Video: ${video_plan}"
    log_info "  Audio: ${audio_plan}"
    log_info "  Container: ${container_plan}"
    log_info "  Subtitles: ${subtitle_plan}"
    log_info "  Attachments: ${attachment_plan}"
    log_info "  HDR: ${hdr_plan}"
    log_info "  Deinterlace: ${deinterlace_plan}"
    log_info "  Retry knobs: mux_queue=${muxing_queue_size}, timestamp_fix=${timestamp_fix}"
}

log_remux_render_plan() {
    local input="$1" video_stream_idx="$2" include_subtitles="$3" include_attachments="$4" muxing_queue_size="$5" timestamp_fix="$6"
    local source_video_codec source_profile source_pix_fmt
    local audio_plan subtitle_plan attachment_plan container_plan

    source_video_codec=$(get_primary_video_codec "$input")
    [[ -z "$source_video_codec" ]] && source_video_codec=$(get_codec "$input")
    source_profile=$(get_primary_video_profile "$input")
    source_pix_fmt=$(get_primary_video_pix_fmt "$input")

    audio_plan=$(describe_audio_plan "$input")
    subtitle_plan=$(describe_subtitle_plan "$input" "$include_subtitles")
    attachment_plan=$(describe_attachment_plan "$include_attachments")

    if output_container_is_mp4; then
        container_plan="MP4 (faststart + hvc1 tag)"
    else
        container_plan="${OUTPUT_CONTAINER^^}"
    fi

    log_info "Pre-flight render params:"
    log_info "  Video: transcode=no (copy stream ${video_stream_idx}: ${source_video_codec:-unknown}, profile=${source_profile:-unknown}, pix_fmt=${source_pix_fmt:-unknown})"
    log_info "  Audio: ${audio_plan}"
    log_info "  Container: ${container_plan}"
    log_info "  Subtitles: ${subtitle_plan}"
    log_info "  Attachments: ${attachment_plan}"
    log_info "  Retry knobs: mux_queue=${muxing_queue_size}, timestamp_fix=${timestamp_fix}"
}

#------------------------------------------------------------------------------
# Build subtitle options
#------------------------------------------------------------------------------
build_subtitle_opts() {
    local input="$1"
    local include_subtitles="$2"
    local -a opts=()
    local subtitle_codec="copy"

    if [[ "$KEEP_SUBTITLES" != true || "$include_subtitles" != true ]]; then
        return
    fi

    if output_container_is_mp4; then
        if has_bitmap_subtitles "$input"; then
            log_warn "  Bitmap subtitles cannot be included in MP4 container" >&2
            return
        fi
        subtitle_codec="mov_text"
    fi

    opts=(-map 0:s? -c:s "$subtitle_codec")
    printf '%s\n' "${opts[*]}"
}

#------------------------------------------------------------------------------
# Remux HEVC sources (copy video, encode audio)
#------------------------------------------------------------------------------
run_remux_with_audio_opts() {
    local input="$1" output="$2" video_stream_idx="$3" err_file="$4" include_subtitles="$5" include_attachments="$6" muxing_queue_size="${7:-4096}" timestamp_fix="${8:-false}"
    shift 8
    local -a audio_opts_array=() subtitle_opts_array=() attachment_opts=() pre_input_opts=() timestamp_opts=() stream_disposition_opts=() container_opts=() tag_opts=()
    local audio_stream_count=0 i mp4_output=false

    # Parse audio opts from remaining args
    read -ra audio_opts_array <<< "$*"

    if output_container_is_mp4; then
        mp4_output=true
        container_opts=(-movflags +faststart)
    else
        container_opts=()
    fi

    # hvc1 tag for compatibility (MP4 only; MKV uses codec IDs natively)
    if [[ "$mp4_output" == true ]]; then
        tag_opts=(-tag:v hvc1)
    else
        tag_opts=()
    fi

    if [[ "$timestamp_fix" == true ]]; then
        pre_input_opts=(-fflags +genpts+discardcorrupt)
        timestamp_opts=(-avoid_negative_ts make_zero)
    else
        pre_input_opts=()
        timestamp_opts=()
    fi

    # Build subtitle options
    local sub_opts_str
    sub_opts_str=$(build_subtitle_opts "$input" "$include_subtitles")
    [[ -n "$sub_opts_str" ]] && read -ra subtitle_opts_array <<< "$sub_opts_str"

    if [[ "$KEEP_ATTACHMENTS" == true && "$include_attachments" == true ]]; then
        if [[ "$mp4_output" == true ]]; then
            attachment_opts=()
        else
            attachment_opts=(-map 0:t? -c:t copy)
        fi
    else
        attachment_opts=()
    fi

    audio_stream_count=$(get_stream_count "a" "$input")
    stream_disposition_opts=(-disposition:v:0 default)
    if [[ "$audio_stream_count" -gt 0 ]]; then
        stream_disposition_opts+=(-disposition:a:0 default)
        for ((i=1; i<audio_stream_count; i++)); do
            stream_disposition_opts+=(-disposition:a:"$i" 0)
        done
    fi

    run_ffmpeg_logged "$err_file" \
        ffmpeg -hide_banner -nostdin -y -loglevel "$FFMPEG_LOGLEVEL" ${FFMPEG_PROGRESS_ARGS[@]+"${FFMPEG_PROGRESS_ARGS[@]}"} \
            -probesize "$FFMPEG_PROBESIZE" -analyzeduration "$FFMPEG_ANALYZEDURATION" -ignore_unknown \
            ${pre_input_opts[@]+"${pre_input_opts[@]}"} \
            -i "$input" \
            -map "0:${video_stream_idx}" ${audio_opts_array[@]+"${audio_opts_array[@]}"} ${subtitle_opts_array[@]+"${subtitle_opts_array[@]}"} ${attachment_opts[@]+"${attachment_opts[@]}"} \
            -dn -max_muxing_queue_size "$muxing_queue_size" -max_interleave_delta 0 \
            -c:v copy \
            ${tag_opts[@]+"${tag_opts[@]}"} \
            ${stream_disposition_opts[@]+"${stream_disposition_opts[@]}"} \
            -map_metadata 0 -map_chapters 0 \
            ${timestamp_opts[@]+"${timestamp_opts[@]}"} \
            ${container_opts[@]+"${container_opts[@]}"} \
            "$output"
}

run_remux_attempt() {
    local input="$1" output="$2" video_stream_idx="$3" err_file="$4" include_attachments="${5:-true}" include_subtitles="${6:-true}" muxing_queue_size="${7:-4096}" timestamp_fix="${8:-false}"
    local audio_opts

    audio_opts=$(build_audio_opts "$input")

    run_remux_with_audio_opts "$input" "$output" "$video_stream_idx" "$err_file" "$include_subtitles" "$include_attachments" "$muxing_queue_size" "$timestamp_fix" $audio_opts
}

#------------------------------------------------------------------------------
# Full transcode (video + audio encode)
#------------------------------------------------------------------------------
run_encode_attempt() {
    local input="$1" output="$2" video_stream_idx="$3" err_file="$4" include_attachments="${5:-true}" include_subtitles="${6:-true}" muxing_queue_size="${7:-4096}" timestamp_fix="${8:-false}"
    local -a audio_opts_array=() subtitle_opts_array=() attachment_opts=() pre_input_opts=() timestamp_opts=() stream_disposition_opts=() container_opts=() tag_opts=() color_opts=()
    local audio_stream_count=0 i mp4_output=false vf_chain

    if output_container_is_mp4; then
        mp4_output=true
        container_opts=(-movflags +faststart)
    else
        container_opts=()
    fi

    # Build audio options
    local audio_opts_str
    audio_opts_str=$(build_audio_opts "$input")
    [[ -n "$audio_opts_str" ]] && read -ra audio_opts_array <<< "$audio_opts_str"

    # Build subtitle options
    local sub_opts_str
    sub_opts_str=$(build_subtitle_opts "$input" "$include_subtitles")
    [[ -n "$sub_opts_str" ]] && read -ra subtitle_opts_array <<< "$sub_opts_str"

    if [[ "$KEEP_ATTACHMENTS" == true && "$include_attachments" == true ]]; then
        if [[ "$mp4_output" == true ]]; then
            attachment_opts=()
        else
            attachment_opts=(-map 0:t? -c:t copy)
        fi
    else
        attachment_opts=()
    fi

    if [[ "$timestamp_fix" == true ]]; then
        pre_input_opts=(-fflags +genpts+discardcorrupt)
        timestamp_opts=(-avoid_negative_ts make_zero)
    else
        pre_input_opts=()
        timestamp_opts=()
    fi

    # Preserve color metadata for HDR passthrough
    local hdr_type
    hdr_type=$(detect_hdr_type "$input")
    if [[ "$hdr_type" == "hdr10" && "$HANDLE_HDR" == "preserve" ]]; then
        local color_meta
        color_meta=$(get_color_metadata "$input")
        IFS=':' read -r ct cp cs <<< "$color_meta"
        [[ "$ct" != "unknown" ]] && color_opts+=(-color_trc "$ct")
        [[ "$cp" != "unknown" ]] && color_opts+=(-color_primaries "$cp")
        [[ "$cs" != "unknown" ]] && color_opts+=(-colorspace "$cs")
    fi

    # hvc1 tag for compatibility (MP4 only; MKV uses codec IDs natively)
    if [[ "$mp4_output" == true ]]; then
        tag_opts=(-tag:v hvc1)
    else
        tag_opts=()
    fi

    audio_stream_count=$(get_stream_count "a" "$input")
    stream_disposition_opts=(-disposition:v:0 default)
    if [[ "$audio_stream_count" -gt 0 ]]; then
        stream_disposition_opts+=(-disposition:a:0 default)
        for ((i=1; i<audio_stream_count; i++)); do
            stream_disposition_opts+=(-disposition:a:"$i" 0)
        done
    fi

    # Build video filter chain
    vf_chain=$(build_video_filter "$input" "$ENCODER_MODE")

    if [[ "$ENCODER_MODE" == "vaapi" ]]; then
        local -a vf_opts=()
        [[ -n "$vf_chain" ]] && vf_opts=(-vf "$vf_chain")

        run_ffmpeg_logged "$err_file" \
            ffmpeg -hide_banner -nostdin -y -loglevel "$FFMPEG_LOGLEVEL" ${FFMPEG_PROGRESS_ARGS[@]+"${FFMPEG_PROGRESS_ARGS[@]}"} \
                -probesize "$FFMPEG_PROBESIZE" -analyzeduration "$FFMPEG_ANALYZEDURATION" -ignore_unknown \
                ${pre_input_opts[@]+"${pre_input_opts[@]}"} \
                -init_hw_device vaapi=va:"$VAAPI_DEVICE" -filter_hw_device va \
                -i "$input" \
                ${vf_opts[@]+"${vf_opts[@]}"} \
                -map "0:${video_stream_idx}" ${audio_opts_array[@]+"${audio_opts_array[@]}"} ${subtitle_opts_array[@]+"${subtitle_opts_array[@]}"} ${attachment_opts[@]+"${attachment_opts[@]}"} \
                -dn -max_muxing_queue_size "$muxing_queue_size" -max_interleave_delta 0 \
                -c:v hevc_vaapi -qp "$VAAPI_QP" -profile:v "$VAAPI_PROFILE" -g "$KEYFRAME_INT" \
                ${tag_opts[@]+"${tag_opts[@]}"} \
                ${color_opts[@]+"${color_opts[@]}"} \
                ${stream_disposition_opts[@]+"${stream_disposition_opts[@]}"} \
                -map_metadata 0 -map_chapters 0 \
                ${timestamp_opts[@]+"${timestamp_opts[@]}"} \
                ${container_opts[@]+"${container_opts[@]}"} \
                "$output"
    else
        local -a vf_opts=()
        if [[ -n "$vf_chain" ]]; then
            vf_opts=(-vf "$vf_chain")
        fi

        run_ffmpeg_logged "$err_file" \
            ffmpeg -hide_banner -nostdin -y -loglevel "$FFMPEG_LOGLEVEL" ${FFMPEG_PROGRESS_ARGS[@]+"${FFMPEG_PROGRESS_ARGS[@]}"} \
                -probesize "$FFMPEG_PROBESIZE" -analyzeduration "$FFMPEG_ANALYZEDURATION" -ignore_unknown \
                ${pre_input_opts[@]+"${pre_input_opts[@]}"} \
                -i "$input" \
                ${vf_opts[@]+"${vf_opts[@]}"} \
                -map "0:${video_stream_idx}" ${audio_opts_array[@]+"${audio_opts_array[@]}"} ${subtitle_opts_array[@]+"${subtitle_opts_array[@]}"} ${attachment_opts[@]+"${attachment_opts[@]}"} \
                -dn -max_muxing_queue_size "$muxing_queue_size" -max_interleave_delta 0 \
                -c:v libx265 -crf "$CPU_CRF" -preset "$CPU_PRESET" \
                -profile:v "$CPU_HEVC_PROFILE" -pix_fmt "$CPU_PIX_FMT" -g "$KEYFRAME_INT" \
                -x265-params "log-level=error:open-gop=0" \
                ${tag_opts[@]+"${tag_opts[@]}"} \
                ${color_opts[@]+"${color_opts[@]}"} \
                ${stream_disposition_opts[@]+"${stream_disposition_opts[@]}"} \
                -map_metadata 0 -map_chapters 0 \
                ${timestamp_opts[@]+"${timestamp_opts[@]}"} \
                ${container_opts[@]+"${container_opts[@]}"} \
                "$output"
    fi
}

#------------------------------------------------------------------------------
# Filename parsing for TV/Movie classification
#------------------------------------------------------------------------------
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
        SHOW_NAME=$(echo "$base" | sed -E 's/[[:space:]._-]*[Ss][0-9]+[Ee][0-9]+.*//' | tr '._' ' ' | sed 's/[[:space:]-]*$//' | xargs)
        [[ -z "$SHOW_NAME" ]] && SHOW_NAME=$(echo "$parent" | sed -E 's/[Ss][0-9]+.*//' | tr '._' ' ' | sed 's/[[:space:]-]*$//' | xargs)
    # Anime: [Group] Name - 05
    elif [[ "$base" =~ ^(\[.+\])?[[:space:]]*(.+)[[:space:]]+-[[:space:]]*([0-9]{1,3})([[:space:]]|\[|v[0-9]|$) ]]; then
        MEDIA_TYPE="tv"
        SEASON="1"
        EPISODE="${BASH_REMATCH[3]}"
        SHOW_NAME=$(echo "${BASH_REMATCH[2]}" | xargs)
    # Episodic: [Group] Show 05 - Title (supports 21' style episode tokens)
    elif [[ "$base" =~ ^(\[.+\][[:space:]]*)?(.+)[[:space:]_.-]+([0-9]{1,3})\'?[[:space:]]*-[[:space:]]*(.+)$ ]]; then
        MEDIA_TYPE="tv"
        SEASON="1"
        EPISODE="${BASH_REMATCH[3]}"
        SHOW_NAME=$(echo "${BASH_REMATCH[2]}" | tr '._' ' ' | xargs)
    # Episodic fallback: 05 - Title (derive show name from parent directory)
    elif [[ "$base" =~ ^([0-9]{1,3})\'?[[:space:]]*-[[:space:]]*(.+)$ ]]; then
        MEDIA_TYPE="tv"
        SEASON="1"
        EPISODE="${BASH_REMATCH[1]}"
        SHOW_NAME=$(echo "$parent" | tr '._' ' ' | xargs)
    # Anime: [Group]Name_Name_01_BD or Name_01
    elif [[ "$base" =~ ^(\[.+\])?(.+)_([0-9]{2,3})(_[^.]*)?$ ]]; then
        MEDIA_TYPE="tv"
        SEASON="1"
        EPISODE="${BASH_REMATCH[3]}"
        SHOW_NAME=$(echo "${BASH_REMATCH[2]}" | tr '_' ' ' | xargs)
    # Movie with year
    elif [[ "$base" =~ (.+)[._[:space:]]\(?((19[0-9]{2}|20[0-9]{2}))\)? ]]; then
        MEDIA_TYPE="movie"
        MOVIE_NAME=$(echo "${BASH_REMATCH[1]}" | tr '._' ' ' | xargs)
        YEAR="${BASH_REMATCH[2]}"
    else
        MEDIA_TYPE="movie"
        MOVIE_NAME=$(echo "$base" | tr '._' ' ' | xargs)
    fi

    # Clean tags
    local tags='720p|1080p|2160p|4K|UHD|WEB-DL|WEBRip|BluRay|BDRip|BD|DVDRip|HDTV|x264|x265|HEVC|H\.?264|H\.?265|AAC|AC3|DTS|DTS-HD|TrueHD|FLAC|EAC3|DD\+?|Atmos|10bit|HDR|HDR10|HDR10\+|DV|DoVi|Dual\.?Audio|MULTI|REMUX|PROPER|REPACK|EMBER|NF|AMZN|DSNP|HMAX|ATVP'
    SHOW_NAME=$(echo "$SHOW_NAME" | sed -E "s/(^|[[:space:]._-])(${tags})([[:space:]._-]|$).*$//I" | sed -E 's/\[[^]]*\]//g' | xargs)
    MOVIE_NAME=$(echo "$MOVIE_NAME" | sed -E "s/(^|[[:space:]._-])(${tags})([[:space:]._-]|$).*$//I" | sed -E 's/\[[^]]*\]//g' | xargs)

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

#------------------------------------------------------------------------------
# Encode a single file with retry logic
#------------------------------------------------------------------------------
encode_file() {
    local input="$1" output="$2" video_stream_idx="${3:-0}"

    mkdir -p "$(dirname "$output")" || { log_error "Can't create dir"; return 1; }

    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY] $input"
        log_info "   -> $output"
        return 0
    fi

    log_info "Encoding: $(basename "$input")"
    log_info "  -> $(basename "$output")"

    local start encode_result=1
    start=$(date +%s)
    local ffmpeg_err
    ffmpeg_err=$(make_temp_file)
    local encode_include_attachments=true
    local encode_include_subtitles=true
    local encode_muxing_queue_size=4096
    local encode_timestamp_fix="$CLEAN_TIMESTAMPS"
    local retry_count=0
    local max_retries=4

    log_encode_render_plan "$input" "$video_stream_idx" "$encode_include_subtitles" "$encode_include_attachments" "$encode_muxing_queue_size" "$encode_timestamp_fix"

    # Try encoding with progressive fallbacks
    while [[ $retry_count -lt $max_retries ]]; do
        if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
            encode_result=0
            break
        fi

        [[ "$STRICT_MODE" == true ]] && break

        ((retry_count++))

        if [[ "$encode_include_attachments" == true ]] && ffmpeg_error_has_attachment_tag_issue "$ffmpeg_err"; then
            log_warn "Retry $retry_count: removing attachments"
            encode_include_attachments=false
            rm -f "$output"
            continue
        fi

        if [[ "$encode_include_subtitles" == true ]] && ffmpeg_error_has_subtitle_mux_issue "$ffmpeg_err"; then
            log_warn "Retry $retry_count: removing subtitles"
            encode_include_subtitles=false
            rm -f "$output"
            continue
        fi

        if [[ "$encode_muxing_queue_size" -lt 16384 ]] && ffmpeg_error_has_mux_queue_overflow "$ffmpeg_err"; then
            log_warn "Retry $retry_count: increasing mux queue to 16384"
            encode_muxing_queue_size=16384
            rm -f "$output"
            continue
        fi

        if [[ "$encode_timestamp_fix" != true ]] && ffmpeg_error_has_timestamp_discontinuity "$ffmpeg_err"; then
            log_warn "Retry $retry_count: enabling timestamp fix"
            encode_timestamp_fix=true
            rm -f "$output"
            continue
        fi

        # No matching fix found, stop retrying
        break
    done

    local elapsed=$(( $(date +%s) - start ))

    if [[ $encode_result -eq 0 && -f "$output" ]]; then
        local in_sz out_sz ratio
        in_sz=$(get_file_size "$input")
        out_sz=$(get_file_size "$output")
        [[ $in_sz -gt 0 ]] && ratio=$((out_sz * 100 / in_sz)) || ratio=0
        log_success "Done in ${elapsed}s (${ratio}%)"
        return 0
    else
        log_error "Failed!"
        if [[ -s "$ffmpeg_err" ]]; then
            log_error "Last ffmpeg output:"
            tail -n 20 "$ffmpeg_err" | sed 's/^/  /'
        fi
        rm -f "$output"
        return 1
    fi
}

#------------------------------------------------------------------------------
# Process all files in input directory
#------------------------------------------------------------------------------
process_files() {
    local -a files=()
    local exts="mkv|mp4|avi|m4v|mov|wmv|flv|webm|ts|m2ts|mpg|mpeg|vob|ogv"

    while IFS= read -r -d '' f; do
        files+=("$f")
    done < <(find "$INPUT_DIR" -type f -regextype posix-extended -iregex ".*\.($exts)$" -print0 | sort -z)

    local total=${#files[@]} current=0 encoded=0 skipped=0 failed=0
    FILE_SUMMARY_ROWS=()

    log_info "Found $total files"
    local profile_label="$CPU_HEVC_PROFILE"
    [[ "$ENCODER_MODE" == "vaapi" ]] && profile_label="$VAAPI_PROFILE"
    log_info "Mode: $ENCODER_MODE (HEVC ${profile_label}), QP/CRF: $([[ $ENCODER_MODE == vaapi ]] && echo $VAAPI_QP || echo $CPU_CRF)"
    log_info "Container: ${OUTPUT_CONTAINER^^}"
    log_info "Audio: AAC passthrough (no AAC->AAC), otherwise encode to AAC ${AUDIO_BITRATE}"
    if output_container_is_mp4; then
        log_info "Compatibility: hvc1 tag for Apple/browser support"
    fi
    [[ "$HANDLE_HDR" == "preserve" ]] && log_info "HDR: Preserve metadata when present"
    [[ "$HANDLE_HDR" == "tonemap" ]] && log_info "HDR: Tonemap to SDR"
    [[ "$DEINTERLACE_AUTO" == true ]] && log_info "Deinterlace: Auto-detect and apply yadif"
    if [[ "$KEEP_SUBTITLES" == true ]]; then
        if output_container_is_mp4; then
            log_info "Subtitles: Text subs only (mov_text for MP4)"
        else
            log_info "Subtitles: Copy all streams"
        fi
    fi
    if [[ "$KEEP_ATTACHMENTS" == true ]] && ! output_container_is_mp4; then
        log_info "Attachments: Copy fonts/images"
    fi
    [[ "$SKIP_HEVC" == true ]] && log_info "HEVC sources: Remux (copy video, encode audio)"
    [[ "$STRICT_MODE" == true ]] && log_info "Retry policy: Strict mode (no auto-retry)"
    echo

    for f in "${files[@]}"; do
        ((current++))

        log_info "[$current/$total] $(basename "$f")"
        local summary_media_type="unknown"
        local summary_output=""
        local summary_audio_action="unknown"
        local summary_note=""

        # Validate file before processing
        if ! validate_input_file "$f"; then
            log_error "Skipping invalid file"
            append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "validate" "unknown" "$summary_audio_action" "failed" "invalid input file"
            ((failed++))
            echo
            continue
        fi

        # Check for video stream
        local video_idx
        video_idx=$(get_primary_video_stream_index "$f")
        if [[ -z "$video_idx" ]]; then
            log_warn "No video stream found, skipping"
            append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "analyze" "none" "$summary_audio_action" "skipped" "no video stream found"
            ((skipped++))
            echo
            continue
        fi

        parse_filename "$(basename "$f")" "$(basename "$(dirname "$f")")"
        local out
        out=$(get_output_path)
        local video_codec video_resolution video_bitrate_label
        video_codec=$(get_primary_video_codec "$f")
        [[ -z "$video_codec" ]] && video_codec=$(get_codec "$f")
        video_resolution=$(get_primary_video_resolution "$f")
        video_bitrate_label=$(get_primary_video_bitrate_label "$f")
        summary_media_type="$MEDIA_TYPE"
        summary_output="$out"
        summary_audio_action=$(describe_audio_plan "$f")

        log_debug "Primary video stream: index=${video_idx}, codec=${video_codec:-unknown}"

        if [[ "$SHOW_FILE_STATS" == true ]]; then
            local hdr_type
            hdr_type=$(detect_hdr_type "$f")
            local hdr_label=""
            [[ "$hdr_type" != "sdr" ]] && hdr_label=" [HDR]"
            local interlace_label=""
            is_interlaced "$f" && interlace_label=" [Interlaced]"
            log_info "  Video: ${video_resolution} | ${video_bitrate_label} | ${video_codec:-unknown}${hdr_label}${interlace_label}"
        fi

        # Check if already HEVC - copy video, encode audio only
        if [[ "$SKIP_HEVC" == true && "$video_codec" == "hevc" ]]; then
            local allow_hevc_remux=true
            local source_profile source_pix_fmt
            source_profile=$(get_primary_video_profile "$f")
            source_pix_fmt=$(get_primary_video_pix_fmt "$f")

            if ! is_edge_safe_hevc_stream "$source_profile" "$source_pix_fmt"; then
                allow_hevc_remux=false
                summary_note="HEVC profile not browser-safe; forced video transcode"
                log_warn "  HEVC profile '${source_profile:-unknown}' not browser-safe; will re-encode"
            fi

            if [[ "$allow_hevc_remux" == true ]]; then
                if [[ "$SKIP_EXISTING" == true && -f "$out" ]]; then
                    log_warn "Skip (exists): $(basename "$out")"
                    append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "remux" "copy" "$summary_audio_action" "skipped" "output exists"
                    ((skipped++))
                    echo
                    continue
                fi

                log_info "Remuxing (copy HEVC, encode AAC): $(basename "$f")"
                log_info "  -> $(basename "$out")"
                mkdir -p "$(dirname "$out")"

                if [[ "$DRY_RUN" == true ]]; then
                    log_success "[DRY] Would remux"
                    append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "remux" "copy" "$summary_audio_action" "dry_run" "would remux"
                    ((encoded++))
                    echo
                    continue
                fi

                local start_rm=$(date +%s)
                local remux_err remux_ok=false
                remux_err=$(make_temp_file)
                local remux_include_subtitles=true
                local remux_include_attachments=true
                local remux_muxing_queue_size=4096
                local remux_timestamp_fix="$CLEAN_TIMESTAMPS"
                local remux_retry_count=0
                local remux_max_retries=4

                log_remux_render_plan "$f" "$video_idx" "$remux_include_subtitles" "$remux_include_attachments" "$remux_muxing_queue_size" "$remux_timestamp_fix"

                while [[ $remux_retry_count -lt $remux_max_retries ]]; do
                    if run_remux_attempt "$f" "$out" "$video_idx" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                        remux_ok=true
                        break
                    fi

                    [[ "$STRICT_MODE" == true ]] && break

                    ((remux_retry_count++))
                    rm -f "$out"

                    if [[ "$remux_include_attachments" == true ]] && ffmpeg_error_has_attachment_tag_issue "$remux_err"; then
                        log_warn "Remux retry $remux_retry_count: skip attachments"
                        remux_include_attachments=false
                        continue
                    fi

                    if [[ "$remux_include_subtitles" == true ]] && ffmpeg_error_has_subtitle_mux_issue "$remux_err"; then
                        log_warn "Remux retry $remux_retry_count: skip subtitles"
                        remux_include_subtitles=false
                        continue
                    fi

                    if [[ "$remux_muxing_queue_size" -lt 16384 ]] && ffmpeg_error_has_mux_queue_overflow "$remux_err"; then
                        log_warn "Remux retry $remux_retry_count: increase queue"
                        remux_muxing_queue_size=16384
                        continue
                    fi

                    if [[ "$remux_timestamp_fix" != true ]] && ffmpeg_error_has_timestamp_discontinuity "$remux_err"; then
                        log_warn "Remux retry $remux_retry_count: fix timestamps"
                        remux_timestamp_fix=true
                        continue
                    fi

                    break
                done

                if [[ "$remux_ok" != true ]]; then
                    log_error "Remux failed"
                    if [[ -s "$remux_err" ]]; then
                        log_error "Last ffmpeg output:"
                        tail -n 20 "$remux_err" | sed 's/^/  /'
                    fi
                    rm -f "$out"
                    append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "remux" "copy" "$summary_audio_action" "failed" "remux failed"
                    ((failed++))
                    echo
                    continue
                fi

                local elapsed_rm=$(( $(date +%s) - start_rm ))
                local in_sz out_sz ratio
                in_sz=$(get_file_size "$f")
                out_sz=$(get_file_size "$out")
                [[ $in_sz -gt 0 ]] && ratio=$((out_sz * 100 / in_sz)) || ratio=100
                log_success "Remuxed in ${elapsed_rm}s (${ratio}% of original)"
                append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "remux" "copy" "$summary_audio_action" "ok" "remuxed"
                ((encoded++))
                echo
                continue
            fi
        fi

        if [[ "$SKIP_EXISTING" == true && -f "$out" ]]; then
            log_warn "Skip (exists)"
            local skip_note="output exists"
            [[ -n "$summary_note" ]] && skip_note="${summary_note}; ${skip_note}"
            append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "encode" "transcode" "$summary_audio_action" "skipped" "$skip_note"
            ((skipped++))
            echo
            continue
        fi

        if encode_file "$f" "$out" "$video_idx"; then
            local encode_note="encoded"
            local encode_status="ok"
            if [[ "$DRY_RUN" == true ]]; then
                encode_note="would encode"
                encode_status="dry_run"
            fi
            [[ -n "$summary_note" ]] && encode_note="${summary_note}; ${encode_note}"
            append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "encode" "transcode" "$summary_audio_action" "$encode_status" "$encode_note"
            ((encoded++))
        else
            local fail_note="encode failed"
            [[ -n "$summary_note" ]] && fail_note="${summary_note}; ${fail_note}"
            append_file_csv_summary "$f" "$summary_output" "$summary_media_type" "encode" "transcode" "$summary_audio_action" "failed" "$fail_note"
            ((failed++))
        fi
        echo
    done

    log_info "=============================="
    log_info "Done: $encoded encoded, $skipped skipped, $failed failed"
    print_file_csv_summary

    return 0
}

#------------------------------------------------------------------------------
# System diagnostics
#------------------------------------------------------------------------------
run_check() {
    log_info "=== System Check ==="

    if command -v ffmpeg &>/dev/null; then
        local ffmpeg_version
        ffmpeg_version=$(ffmpeg -version 2>/dev/null | head -1)
        log_success "ffmpeg: ${ffmpeg_version:-unknown}"
    else
        log_error "ffmpeg not found"
    fi

    log_info "HEVC encoders:"
    local hevc_encoders
    hevc_encoders=$(ffmpeg -hide_banner -encoders 2>/dev/null | grep -E "hevc|265" || true)
    if [[ -n "$hevc_encoders" ]]; then
        printf '%s\n' "$hevc_encoders"
    else
        log_warn "No HEVC-related encoders reported by ffmpeg"
    fi

    local dev
    dev=$(get_first_render_device || true)
    if [[ -n "$dev" ]]; then
        log_info "Testing VAAPI on $dev..."
        if ffmpeg -hide_banner -nostdin -init_hw_device vaapi=va:"$dev" \
                -f lavfi -i color=black:s=256x256:d=0.1 \
                -vf 'format=p010,hwupload' -c:v hevc_vaapi -profile:v main10 -f null - 2>/dev/null; then
            log_success "VAAPI works (main10)"
        elif ffmpeg -hide_banner -nostdin -init_hw_device vaapi=va:"$dev" \
                -f lavfi -i color=black:s=256x256:d=0.1 \
                -vf 'format=nv12,hwupload' -c:v hevc_vaapi -profile:v main -f null - 2>/dev/null; then
            log_success "VAAPI works (main/8-bit only)"
        else
            log_error "VAAPI failed"
        fi
    else
        log_warn "No VAAPI device found"
    fi

    log_info "Testing CPU x265..."
    if ffmpeg -hide_banner -nostdin -f lavfi -i color=black:s=256x256:d=0.1 -c:v libx265 -f null - 2>/dev/null; then
        log_success "CPU x265 works"
    else
        log_error "CPU x265 failed"
    fi

    log_info "Testing AAC encoder..."
    if ffmpeg -hide_banner -nostdin -f lavfi -i sine=frequency=1000:duration=0.1 -c:a aac -f null - 2>/dev/null; then
        log_success "AAC encoder works"
    else
        log_error "AAC encoder failed"
    fi
}

#------------------------------------------------------------------------------
# Entrypoint
#------------------------------------------------------------------------------
main() {
    init_colors
    parse_args "$@"
    init_colors  # Re-init after --color/--no-color parsed
    print_banner

    if [[ "$CHECK_ONLY" == true ]]; then
        run_check
        exit 0
    fi

    [[ ! -d "$INPUT_DIR" ]] && { log_error "Input not found: $INPUT_DIR"; exit 1; }
    mkdir -p "$OUTPUT_DIR"

    local input_abs output_abs
    input_abs=$(cd "$INPUT_DIR" && pwd -P) || { log_error "Cannot resolve input path: $INPUT_DIR"; exit 1; }
    output_abs=$(cd "$OUTPUT_DIR" && pwd -P) || { log_error "Cannot resolve output path: $OUTPUT_DIR"; exit 1; }
    if [[ "$output_abs" == "$input_abs" || "$output_abs" == "$input_abs/"* ]]; then
        log_error "Output directory must not be inside input directory"
        log_error "Choose an output path outside: $INPUT_DIR"
        exit 1
    fi

    log_info "=== Muxmaster v${SCRIPT_VERSION} ==="
    log_info "In:  $INPUT_DIR"
    log_info "Out: $OUTPUT_DIR"
    [[ "$DRY_RUN" == true ]] && log_warn "DRY RUN"
    echo

    check_deps
    process_files
}

main "$@"

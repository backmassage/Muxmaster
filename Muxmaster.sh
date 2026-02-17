#!/bin/bash
#===============================================================================
# Muxmaster Media Library Encoder
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
OUTPUT_CONTAINER="mp4"
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
SKIP_HEVC_EXPLICIT=false
AUTO_EDGE_SAFE_HEVC_REENCODE=false
ALLOW_UNSAFE_VAAPI_MP4=false
AUTO_CPU_MODE_FOR_MP4_SAFETY=false
CLEAN_METADATA=true
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
FORCE_HEVC_10BIT=false

INPUT_DIR=""
OUTPUT_DIR=""
SCRIPT_NAME="$(basename "$0")"
SCRIPT_VERSION="1.2"

# ANSI color palette (initialized by init_colors)
RED=""; GREEN=""; YELLOW=""; BLUE=""; CYAN=""; NC=""

# Initialize color variables according to --color/--no-color/auto rules.
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
        # Use bright variants for stronger terminal contrast/visibility.
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

# Print startup banner.
print_banner() {
    local banner_color="$CYAN"
    if [[ -n "$CYAN" ]]; then
        banner_color=$'\033[1;95m'
        printf '%b\n' "$banner_color"
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

log_line() {
    local level="$1"
    local color="$2"
    local text="$3"
    local ts="[$(date '+%Y-%m-%d %H:%M:%S')]"
    local stream_fd=1

    # Emit errors on stderr for better CLI/pipeline behavior.
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
log_debug()   { [[ "$VERBOSE" == true ]] && log_line "DEBUG" "$CYAN" "$1"; }

# Print CLI help and exit with the requested code.
usage() {
    local exit_code="${1:-0}"
    local usage_stream=1
    [[ "$exit_code" -ne 0 ]] && usage_stream=2

    cat >&"$usage_stream" << EOF
Muxmaster v$SCRIPT_VERSION

Usage: $SCRIPT_NAME [OPTIONS] <input_dir> <output_dir>

Options:
  -m, --mode <vaapi|cpu>    Encoder mode (default: vaapi; MP4 auto-switches to CPU unless --allow-unsafe-vaapi-mp4)
  -q, --quality <value>     QP for VAAPI, CRF for CPU (default: 19, lower=better)
  -p, --preset <preset>     CPU preset (default: slow)
  -d, --dry-run             Preview only
  --skip-hevc               HEVC files: copy video, encode audio only
  --no-skip-hevc            Re-encode HEVC video instead of remuxing it
  --clean-metadata          Strip container metadata/chapters (default: on)
  --keep-metadata           Preserve source container metadata/chapters
  --show-fps                Show live ffmpeg encoding FPS/speed (default: on)
  --no-fps                  Disable live ffmpeg FPS/speed progress
  --no-stats                Hide per-file source video stats section
  --no-subs                 Do not process subtitle streams
  --no-attachments          Do not include attachment streams (fonts/images)
  --strict                  Disable automatic ffmpeg retry fallbacks
  --clean-timestamps        Enable proactive timestamp regeneration (default: on)
  --no-clean-timestamps     Disable proactive timestamp regeneration
  --match-audio-layout      Force audio layout normalization to stereo (default: on)
  --no-match-audio-layout   Disable explicit audio layout normalization
  --hevc-10bit              Force HEVC main10 10-bit output (test mode)
  --hevc-8bit               Force HEVC main 8-bit output
  --allow-unsafe-vaapi-mp4  Keep VAAPI mode for MP4 outputs (not recommended)
  -f, --force               Overwrite existing output files
  -l, --log <path>          Write plain logs to file
  --                        End options parsing
  --color                   Force colored logs
  --no-color                Disable colored logs
  -v, --verbose             Verbose output (includes ffmpeg progress/details)
  -c, --check               System diagnostics
  -V, --version             Print script version and exit
  -h, --help                Help

Encoding defaults: MP4 outputs prefer CPU encode safety (VAAPI requires --allow-unsafe-vaapi-mp4), HEVC main for MP4 edge safety (use --hevc-10bit to test main10), QP/CRF 19, all audio -> AAC stereo 224k (strict, no audio-copy fallback), output container MP4, source/default keyframe cadence (not forced), clean container metadata, proactive timestamp cleanup + anomaly retries
EOF
    exit "$exit_code"
}

show_version() {
    printf '%s v%s\n' "$SCRIPT_NAME" "$SCRIPT_VERSION"
}

# Ensure options that require values never trigger a shift-loop.
require_option_value() {
    local opt="$1"
    local value="${2-}"
    if [[ -z "$value" ]]; then
        log_error "Option '$opt' requires a value"
        usage 1
    fi
}

# Remove trailing slashes from directory args while preserving root (/).
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

# Parse and validate CLI arguments, then derive runtime ffmpeg log settings.
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
            -d|--dry-run) DRY_RUN=true; shift ;;
            --skip-hevc) SKIP_HEVC=true; SKIP_HEVC_EXPLICIT=true; shift ;;
            --no-skip-hevc) SKIP_HEVC=false; SKIP_HEVC_EXPLICIT=true; shift ;;
            --clean-metadata) CLEAN_METADATA=true; shift ;;
            --keep-metadata) CLEAN_METADATA=false; shift ;;
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
            --hevc-10bit) FORCE_HEVC_10BIT=true; shift ;;
            --hevc-8bit) FORCE_HEVC_10BIT=false; shift ;;
            --allow-unsafe-vaapi-mp4) ALLOW_UNSAFE_VAAPI_MP4=true; shift ;;
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
            -*) log_error "Unknown: $1"; usage 1 ;;
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

    if output_container_is_mp4; then
        # Many edge/browser decode stacks are far more stable with CPU x265 output
        # than VAAPI HEVC output, even when the file appears structurally valid.
        if [[ "$ENCODER_MODE" == "vaapi" && "$ALLOW_UNSAFE_VAAPI_MP4" != true ]]; then
            ENCODER_MODE="cpu"
            AUTO_CPU_MODE_FOR_MP4_SAFETY=true
        fi
    fi

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

    if output_container_is_mp4; then
        # Edge-safe MP4 defaults: prefer HEVC main/8-bit and avoid copying unknown HEVC bitstreams.
        if [[ "$FORCE_HEVC_10BIT" == true ]]; then
            CPU_HEVC_PROFILE="main10"
            CPU_PIX_FMT="yuv420p10le"
        else
            CPU_HEVC_PROFILE="main"
            CPU_PIX_FMT="yuv420p"
        fi

        if [[ "$SKIP_HEVC_EXPLICIT" != true ]]; then
            SKIP_HEVC=false
            AUTO_EDGE_SAFE_HEVC_REENCODE=true
        fi
    else
        CPU_HEVC_PROFILE="main10"
        CPU_PIX_FMT="yuv420p10le"
    fi
}

# Probe whether a specific VAAPI HEVC profile+format combo works.
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

# Return first available VAAPI render device path.
get_first_render_device() {
    local dev
    for dev in /dev/dri/renderD*; do
        [[ -e "$dev" ]] || continue
        printf '%s\n' "$dev"
        return 0
    done
    return 1
}

# Return success when output container is MP4.
output_container_is_mp4() {
    [[ "${OUTPUT_CONTAINER,,}" == "mp4" ]]
}

# Validate required tools and confirm selected encoder path is usable.
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
        vaapi_err=$(mktemp)
        if output_container_is_mp4; then
            if [[ "$FORCE_HEVC_10BIT" == true ]]; then
                if test_vaapi_profile "p010" "main10" "$vaapi_err"; then
                    VAAPI_SW_FORMAT="p010"
                    VAAPI_PROFILE="main10"
                    log_warn "VAAPI ready: $VAAPI_DEVICE (HEVC main10 test mode for MP4)"
                else
                    log_error "VAAPI main10 unavailable for requested --hevc-10bit mode"
                    [[ "$VERBOSE" == true ]] && sed 's/^/  /' "$vaapi_err"
                    rm -f "$vaapi_err"
                    exit 1
                fi
            elif test_vaapi_profile "nv12" "main" "$vaapi_err"; then
                VAAPI_SW_FORMAT="nv12"
                VAAPI_PROFILE="main"
                log_success "VAAPI ready: $VAAPI_DEVICE (HEVC main, edge-safe MP4)"
            elif test_vaapi_profile "p010" "main10" "$vaapi_err"; then
                VAAPI_SW_FORMAT="p010"
                VAAPI_PROFILE="main10"
                log_warn "VAAPI HEVC main unavailable; falling back to main10 (may decode poorly on some Edge systems)"
                log_success "VAAPI ready: $VAAPI_DEVICE (HEVC main10)"
            else
                log_error "VAAPI test failed"
                [[ "$VERBOSE" == true ]] && sed 's/^/  /' "$vaapi_err"
                rm -f "$vaapi_err"
                exit 1
            fi
        else
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
                rm -f "$vaapi_err"
                exit 1
            fi
        fi
        rm -f "$vaapi_err"
    else
        if ! ffmpeg -hide_banner -nostdin -loglevel error \
                -f lavfi -i color=black:s=256x256:d=0.1 \
                -c:v libx265 -f null - > /dev/null 2>&1; then
            log_error "CPU mode selected but libx265 is unavailable"
            exit 1
        fi
    fi
}

# Return codec name for the first video stream (simple fallback helper).
get_codec() {
    ffprobe -v error -select_streams v:0 -show_entries stream=codec_name \
        -of default=noprint_wrappers=1:nokey=1 "$1" 2>/dev/null | sed -n '1p'
}

# Return success if at least one audio stream exists.
has_audio_stream() {
    local count
    count=$(get_stream_count "a" "$1")
    [[ "$count" -gt 0 ]]
}

# Return success if at least one subtitle stream exists.
has_subtitle_stream() {
    local count
    count=$(get_stream_count "s" "$1")
    [[ "$count" -gt 0 ]]
}

# Return stream count for a selector (e.g., a, s, v).
get_stream_count() {
    local selector="$1"
    local input="$2"
    local count

    count=$(ffprobe -v error -select_streams "$selector" -show_entries stream=index \
        -of csv=p=0 "$input" 2>/dev/null | sed '/^[[:space:]]*$/d' | wc -l | tr -d '[:space:]')
    [[ "$count" =~ ^[0-9]+$ ]] || count=0
    printf '%s\n' "$count"
}

# Return first tag value for a specific stream and tag key.
get_stream_tag_value() {
    local input="$1"
    local selector="$2"
    local index="$3"
    local tag_key="$4"

    ffprobe -v error -select_streams "${selector}:${index}" \
        -show_entries stream_tags="${tag_key}" \
        -of default=noprint_wrappers=1:nokey=1 "$input" 2>/dev/null | sed -n '1p'
}

# Return success when handler_name is a generic container default.
is_generic_handler_name() {
    case "$1" in
        ""|SoundHandler|VideoHandler|SubtitleHandler)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

# Return the first non-attached-pic video stream index.
# This avoids selecting cover art as the "main" video stream.
get_primary_video_stream_index() {
    local idx codec attached
    while IFS='|' read -r idx codec attached; do
        [[ -z "$idx" ]] && continue
        # Skip cover/attached-pic streams and keep the first real video stream
        [[ "$attached" != "1" ]] && { echo "$idx"; return 0; }
    done < <(ffprobe -v error -select_streams v \
        -show_entries stream=index,codec_name:stream_disposition=attached_pic \
        -of compact=p=0:nk=1 "$1" 2>/dev/null)

    echo "0"
}

# Return codec name for the first non-attached-pic video stream.
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

# Format bitrate in bits/s as a readable label.
format_bitrate_label() {
    local bitrate_bps="$1"

    if [[ "$bitrate_bps" =~ ^[0-9]+$ && "$bitrate_bps" -gt 0 ]]; then
        printf '%d kb/s\n' "$(((bitrate_bps + 500) / 1000))"
    else
        printf 'unknown\n'
    fi
}

# Return WxH for the first non-attached-pic video stream.
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

# Return bitrate (bits/s) for first non-attached-pic video stream, with
# fallback to container bitrate when stream bitrate is unavailable.
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
        -of default=noprint_wrappers=1:nokey=1 "$1" 2>/dev/null | sed -n '1p')
    [[ "$format_bitrate" =~ ^[0-9]+$ && "$format_bitrate" -gt 0 ]] && { echo "$format_bitrate"; return 0; }

    echo ""
}

# Return readable video bitrate label for the primary video stream.
get_primary_video_bitrate_label() {
    local bitrate_bps
    bitrate_bps=$(get_primary_video_bitrate_bps "$1")
    format_bitrate_label "$bitrate_bps"
}

# Execute ffmpeg and capture stderr to an error file.
# In verbose mode (or --show-fps), stderr is mirrored to terminal.
run_ffmpeg_logged() {
    local err_file="$1"
    shift

    if [[ "$VERBOSE" == true || "$SHOW_FFMPEG_FPS" == true ]]; then
        "$@" 2> >(tee "$err_file" >&2)
    else
        "$@" 2>"$err_file"
    fi
}

# Return success when ffmpeg output reports attachment metadata/tag issues.
ffmpeg_error_has_attachment_tag_issue() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eq 'Attachment stream [0-9]+ has no (filename|mimetype) tag' "$err_file"
}

# Return success when ffmpeg output reports subtitle stream mux/copy issues.
ffmpeg_error_has_subtitle_mux_issue() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eqi 'Subtitle codec .* is not supported|Could not find tag for codec .* in stream .*subtitle|Error initializing output stream .*subtitle|Error while opening encoder for output stream .*subtitle|Subtitle encoding currently only possible from text to text or bitmap to bitmap' "$err_file"
}

# Return success when ffmpeg reports mux queue overflow.
ffmpeg_error_has_mux_queue_overflow() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eq 'Too many packets buffered for output stream' "$err_file"
}

# Return success when ffmpeg reports timestamp/PTS ordering anomalies.
ffmpeg_error_has_timestamp_discontinuity() {
    local err_file="$1"
    [[ -s "$err_file" ]] || return 1
    grep -Eqi 'Non-monotonous DTS|non monotonically increasing dts|invalid, non monotonically increasing dts|DTS .*out of order|PTS .*out of order|pts has no value|missing PTS|Timestamps are unset' "$err_file"
}

# Core remux executor used by skip-hevc flow.
# - audio options are injected by caller
# - subtitle/attachment copying follows KEEP_* toggles
# - metadata_mode supports "keep" or "strip"
run_remux_with_audio_opts() {
    local input="$1" output="$2" video_stream_idx="$3" metadata_mode="$4" err_file="$5" include_subtitles="$6" include_attachments="$7" muxing_queue_size="${8:-4096}" timestamp_fix="${9:-false}"
    shift 9
    local -a audio_opts=("$@") metadata_opts subtitle_opts attachment_opts pre_input_opts timestamp_opts stream_metadata_opts stream_disposition_opts container_opts
    local audio_stream_count subtitle_stream_count i subtitle_codec="copy" mp4_output=false language_tag title_tag handler_name_tag

    if output_container_is_mp4; then
        mp4_output=true
        subtitle_codec="mov_text"
        container_opts=(-movflags +faststart+use_metadata_tags)
    else
        container_opts=()
    fi

    if [[ "$timestamp_fix" == true ]]; then
        pre_input_opts=(-fflags +genpts)
        timestamp_opts=(-avoid_negative_ts make_zero)
    else
        pre_input_opts=()
        timestamp_opts=()
    fi

    if [[ "$metadata_mode" == "strip" ]]; then
        metadata_opts=(-map_metadata -1 -map_chapters -1)
    else
        metadata_opts=(-map_metadata 0 -map_chapters 0)
    fi

    if [[ "$KEEP_SUBTITLES" == true && "$include_subtitles" == true ]]; then
        subtitle_opts=(-map 0:s? -c:s "$subtitle_codec")
    else
        subtitle_opts=()
    fi

    if [[ "$KEEP_ATTACHMENTS" == true && "$include_attachments" == true ]]; then
        if [[ "$mp4_output" == true ]]; then
            # MP4 does not support MKV-style attachment streams (fonts/images).
            attachment_opts=()
        else
            attachment_opts=(-map 0:t? -c:t copy)
        fi
    else
        attachment_opts=()
    fi

    # Preserve per-stream metadata (track titles/language tags) for mapped audio/subtitle streams.
    stream_metadata_opts=()
    stream_disposition_opts=()
    audio_stream_count=$(get_stream_count "a" "$input")
    if [[ "$audio_stream_count" -gt 0 ]]; then
        for ((i=0; i<audio_stream_count; i++)); do
            language_tag=$(get_stream_tag_value "$input" "a" "$i" "language")
            title_tag=$(get_stream_tag_value "$input" "a" "$i" "title")
            handler_name_tag=$(get_stream_tag_value "$input" "a" "$i" "handler_name")
            if [[ -z "$title_tag" && -n "$handler_name_tag" ]] && ! is_generic_handler_name "$handler_name_tag"; then
                title_tag="$handler_name_tag"
            fi
            [[ -n "$language_tag" ]] && stream_metadata_opts+=(-metadata:s:a:"$i" "language=$language_tag")
            if [[ -n "$title_tag" ]]; then
                if [[ "$mp4_output" == true ]]; then
                    stream_metadata_opts+=(-metadata:s:a:"$i" "handler_name=$title_tag")
                else
                    stream_metadata_opts+=(-metadata:s:a:"$i" "title=$title_tag")
                fi
            fi

            if [[ "$i" -eq 0 ]]; then
                stream_disposition_opts+=(-disposition:a:"$i" default)
            else
                stream_disposition_opts+=(-disposition:a:"$i" 0)
            fi
        done
    fi

    if [[ "$KEEP_SUBTITLES" == true && "$include_subtitles" == true ]]; then
        subtitle_stream_count=$(get_stream_count "s" "$input")
        if [[ "$subtitle_stream_count" -gt 0 ]]; then
            for ((i=0; i<subtitle_stream_count; i++)); do
                language_tag=$(get_stream_tag_value "$input" "s" "$i" "language")
                title_tag=$(get_stream_tag_value "$input" "s" "$i" "title")
                handler_name_tag=$(get_stream_tag_value "$input" "s" "$i" "handler_name")
                if [[ -z "$title_tag" && -n "$handler_name_tag" ]] && ! is_generic_handler_name "$handler_name_tag"; then
                    title_tag="$handler_name_tag"
                fi
                [[ -n "$language_tag" ]] && stream_metadata_opts+=(-metadata:s:s:"$i" "language=$language_tag")
                if [[ -n "$title_tag" ]]; then
                    if [[ "$mp4_output" == true ]]; then
                        stream_metadata_opts+=(-metadata:s:s:"$i" "handler_name=$title_tag")
                    else
                        stream_metadata_opts+=(-metadata:s:s:"$i" "title=$title_tag")
                    fi
                fi
            done
        fi
    fi

    # Keep all MP4 subtitle tracks non-default to reduce web-player
    # track toggle edge cases during playback.
    if [[ "$mp4_output" == true && "$subtitle_stream_count" -gt 0 ]]; then
        stream_disposition_opts+=(-disposition:s 0)
    fi

    run_ffmpeg_logged "$err_file" \
        ffmpeg -hide_banner -nostdin -y -loglevel "$FFMPEG_LOGLEVEL" "${FFMPEG_PROGRESS_ARGS[@]}" \
            -probesize "$FFMPEG_PROBESIZE" -analyzeduration "$FFMPEG_ANALYZEDURATION" -ignore_unknown \
            "${pre_input_opts[@]}" \
            -i "$input" \
            -map "0:${video_stream_idx}" "${audio_opts[@]}" "${subtitle_opts[@]}" "${attachment_opts[@]}" \
            -dn -max_muxing_queue_size "$muxing_queue_size" -max_interleave_delta 0 \
            -c:v copy \
            "${metadata_opts[@]}" \
            "${stream_metadata_opts[@]}" \
            "${stream_disposition_opts[@]}" \
            "${timestamp_opts[@]}" \
            "${container_opts[@]}" \
            "$output"
}

# Build remux audio args for strict AAC mode:
# - all audio tracks -> AAC
run_remux_attempt() {
    local input="$1" output="$2" video_stream_idx="$3" metadata_mode="$4" err_file="$5" include_attachments="${6:-true}" include_subtitles="${7:-true}" muxing_queue_size="${8:-4096}" timestamp_fix="${9:-false}"
    local -a audio_opts

    if has_audio_stream "$input"; then
        audio_opts=(-map 0:a -c:a aac -ac "$AUDIO_CHANNELS" -ar 48000 -b:a "$AUDIO_BITRATE")
        if [[ "$MATCH_AUDIO_LAYOUT" == true ]]; then
            # Normalize layout/rate and regenerate stable audio frame timing for browser renderers.
            audio_opts+=(-filter:a "aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=48000:channel_layouts=stereo")
        fi
    else
        audio_opts=(-an)
    fi

    run_remux_with_audio_opts "$input" "$output" "$video_stream_idx" "$metadata_mode" "$err_file" "$include_subtitles" "$include_attachments" "$muxing_queue_size" "$timestamp_fix" "${audio_opts[@]}"
}

# Core transcode executor for non-remux flow.
# Video is encoded to HEVC; audio is encoded to AAC for all tracks, and
# subtitles/attachments are preserved by default.
run_encode_attempt() {
    local input="$1" output="$2" video_stream_idx="$3" err_file="$4" include_attachments="${5:-true}" metadata_mode="${6:-}" include_subtitles="${7:-true}" muxing_queue_size="${8:-4096}" timestamp_fix="${9:-false}"
    local -a audio_opts subtitle_opts attachment_opts metadata_opts pre_input_opts timestamp_opts stream_metadata_opts stream_disposition_opts container_opts
    local audio_stream_count subtitle_stream_count i subtitle_codec="copy" mp4_output=false language_tag title_tag handler_name_tag

    if output_container_is_mp4; then
        mp4_output=true
        subtitle_codec="mov_text"
        container_opts=(-movflags +faststart+use_metadata_tags)
    else
        container_opts=()
    fi

    if has_audio_stream "$input"; then
        audio_opts=(-map 0:a -c:a aac -ac "$AUDIO_CHANNELS" -ar 48000 -b:a "$AUDIO_BITRATE")
        if [[ "$MATCH_AUDIO_LAYOUT" == true ]]; then
            # Normalize layout/rate and regenerate stable audio frame timing for browser renderers.
            audio_opts+=(-filter:a "aresample=async=1:first_pts=0:min_hard_comp=0.100,aformat=sample_rates=48000:channel_layouts=stereo")
        fi
    else
        audio_opts=(-an)
    fi

    if [[ "$KEEP_SUBTITLES" == true && "$include_subtitles" == true ]]; then
        subtitle_opts=(-map 0:s? -c:s "$subtitle_codec")
    else
        subtitle_opts=()
    fi

    if [[ "$KEEP_ATTACHMENTS" == true && "$include_attachments" == true ]]; then
        if [[ "$mp4_output" == true ]]; then
            # MP4 does not support MKV-style attachment streams (fonts/images).
            attachment_opts=()
        else
            attachment_opts=(-map 0:t? -c:t copy)
        fi
    else
        attachment_opts=()
    fi

    if [[ -z "$metadata_mode" ]]; then
        [[ "$CLEAN_METADATA" == true ]] && metadata_mode="strip" || metadata_mode="keep"
    fi

    if [[ "$metadata_mode" == "strip" ]]; then
        metadata_opts=(-map_metadata -1 -map_chapters -1)
    else
        metadata_opts=(-map_metadata 0 -map_chapters 0)
    fi

    if [[ "$timestamp_fix" == true ]]; then
        pre_input_opts=(-fflags +genpts)
        timestamp_opts=(-avoid_negative_ts make_zero)
    else
        pre_input_opts=()
        timestamp_opts=()
    fi

    # Preserve per-stream metadata (track titles/language tags) for mapped audio/subtitle streams.
    stream_metadata_opts=()
    stream_disposition_opts=()
    audio_stream_count=$(get_stream_count "a" "$input")
    if [[ "$audio_stream_count" -gt 0 ]]; then
        for ((i=0; i<audio_stream_count; i++)); do
            language_tag=$(get_stream_tag_value "$input" "a" "$i" "language")
            title_tag=$(get_stream_tag_value "$input" "a" "$i" "title")
            handler_name_tag=$(get_stream_tag_value "$input" "a" "$i" "handler_name")
            if [[ -z "$title_tag" && -n "$handler_name_tag" ]] && ! is_generic_handler_name "$handler_name_tag"; then
                title_tag="$handler_name_tag"
            fi
            [[ -n "$language_tag" ]] && stream_metadata_opts+=(-metadata:s:a:"$i" "language=$language_tag")
            if [[ -n "$title_tag" ]]; then
                if [[ "$mp4_output" == true ]]; then
                    stream_metadata_opts+=(-metadata:s:a:"$i" "handler_name=$title_tag")
                else
                    stream_metadata_opts+=(-metadata:s:a:"$i" "title=$title_tag")
                fi
            fi

            if [[ "$i" -eq 0 ]]; then
                stream_disposition_opts+=(-disposition:a:"$i" default)
            else
                stream_disposition_opts+=(-disposition:a:"$i" 0)
            fi
        done
    fi

    if [[ "$KEEP_SUBTITLES" == true && "$include_subtitles" == true ]]; then
        subtitle_stream_count=$(get_stream_count "s" "$input")
        if [[ "$subtitle_stream_count" -gt 0 ]]; then
            for ((i=0; i<subtitle_stream_count; i++)); do
                language_tag=$(get_stream_tag_value "$input" "s" "$i" "language")
                title_tag=$(get_stream_tag_value "$input" "s" "$i" "title")
                handler_name_tag=$(get_stream_tag_value "$input" "s" "$i" "handler_name")
                if [[ -z "$title_tag" && -n "$handler_name_tag" ]] && ! is_generic_handler_name "$handler_name_tag"; then
                    title_tag="$handler_name_tag"
                fi
                [[ -n "$language_tag" ]] && stream_metadata_opts+=(-metadata:s:s:"$i" "language=$language_tag")
                if [[ -n "$title_tag" ]]; then
                    if [[ "$mp4_output" == true ]]; then
                        stream_metadata_opts+=(-metadata:s:s:"$i" "handler_name=$title_tag")
                    else
                        stream_metadata_opts+=(-metadata:s:s:"$i" "title=$title_tag")
                    fi
                fi
            done
        fi
    fi

    if [[ "$mp4_output" == true && "$subtitle_stream_count" -gt 0 ]]; then
        stream_disposition_opts+=(-disposition:s 0)
    fi

    if [[ "$ENCODER_MODE" == "vaapi" ]]; then
        run_ffmpeg_logged "$err_file" \
            ffmpeg -hide_banner -nostdin -y -loglevel "$FFMPEG_LOGLEVEL" "${FFMPEG_PROGRESS_ARGS[@]}" \
                -probesize "$FFMPEG_PROBESIZE" -analyzeduration "$FFMPEG_ANALYZEDURATION" -ignore_unknown \
                "${pre_input_opts[@]}" \
                -init_hw_device vaapi=va:"$VAAPI_DEVICE" -filter_hw_device va \
                -i "$input" -vf "format=${VAAPI_SW_FORMAT},hwupload" \
                -map "0:${video_stream_idx}" "${audio_opts[@]}" "${subtitle_opts[@]}" "${attachment_opts[@]}" \
                -dn -max_muxing_queue_size "$muxing_queue_size" -max_interleave_delta 0 \
                -c:v hevc_vaapi -qp "$VAAPI_QP" -profile:v "$VAAPI_PROFILE" \
                "${metadata_opts[@]}" \
                "${stream_metadata_opts[@]}" \
                "${stream_disposition_opts[@]}" \
                "${timestamp_opts[@]}" \
                "${container_opts[@]}" \
                "$output"
    else
        run_ffmpeg_logged "$err_file" \
            ffmpeg -hide_banner -nostdin -y -loglevel "$FFMPEG_LOGLEVEL" "${FFMPEG_PROGRESS_ARGS[@]}" \
                -probesize "$FFMPEG_PROBESIZE" -analyzeduration "$FFMPEG_ANALYZEDURATION" -ignore_unknown \
                "${pre_input_opts[@]}" \
                -i "$input" \
                -map "0:${video_stream_idx}" "${audio_opts[@]}" "${subtitle_opts[@]}" "${attachment_opts[@]}" \
                -dn -max_muxing_queue_size "$muxing_queue_size" -max_interleave_delta 0 \
                -c:v libx265 -crf "$CPU_CRF" -preset "$CPU_PRESET" \
                -profile:v "$CPU_HEVC_PROFILE" -pix_fmt "$CPU_PIX_FMT" \
                -x265-params log-level=error \
                "${metadata_opts[@]}" \
                "${stream_metadata_opts[@]}" \
                "${stream_disposition_opts[@]}" \
                "${timestamp_opts[@]}" \
                "${container_opts[@]}" \
                "$output"
    fi
}

# Parse filename and infer library classification (TV vs movie) with best-effort
# pattern matching for common release naming styles.
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
    elif [[ "$base" =~ (.+)[._[:space:]]\(?((19[0-9]{2}|20[0-9]{2}))\)? ]]; then
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

# Build destination path from parsed media metadata.
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

# Transcode one source file into its output path.
# AAC conversion is strict: no source-audio copy fallback on failure.
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
    
    local start result
    start=$(date +%s)
    local ffmpeg_err
    ffmpeg_err=$(mktemp)
    local encode_metadata_mode="strip"
    local encode_include_attachments=true
    local encode_include_subtitles=true
    local encode_muxing_queue_size=4096
    local encode_timestamp_fix="$CLEAN_TIMESTAMPS"
    [[ "$CLEAN_METADATA" == false ]] && encode_metadata_mode="keep"

    if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_metadata_mode" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
        result=0
    else
        result=$?

        if [[ "$STRICT_MODE" != true ]]; then
            if [[ "$encode_metadata_mode" == "keep" ]]; then
                log_warn "Encode retry: switching to clean metadata mode"
                encode_metadata_mode="strip"
                rm -f "$output"
                if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_metadata_mode" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
                    result=0
                else
                    result=$?
                fi
            fi

            if [[ "$result" -ne 0 && "$KEEP_ATTACHMENTS" == true ]] && ffmpeg_error_has_attachment_tag_issue "$ffmpeg_err"; then
                log_warn "Encode retry: source attachment tag issue; retrying without attachments"
                rm -f "$output"
                encode_include_attachments=false

                if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_metadata_mode" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
                    result=0
                else
                    result=$?
                fi
            fi

            if [[ "$result" -ne 0 && "$KEEP_SUBTITLES" == true && "$encode_include_subtitles" == true ]] && ffmpeg_error_has_subtitle_mux_issue "$ffmpeg_err"; then
                log_warn "Encode retry: subtitle stream mux issue; retrying without subtitles"
                rm -f "$output"
                encode_include_subtitles=false

                if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_metadata_mode" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
                    result=0
                else
                    result=$?
                fi
            fi

            if [[ "$result" -ne 0 && "$encode_muxing_queue_size" -lt 16384 ]] && ffmpeg_error_has_mux_queue_overflow "$ffmpeg_err"; then
                log_warn "Encode retry: increasing mux queue size to 16384"
                rm -f "$output"
                encode_muxing_queue_size=16384

                if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_metadata_mode" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
                    result=0
                else
                    result=$?
                fi
            fi

            if [[ "$result" -ne 0 && "$encode_timestamp_fix" != true ]] && ffmpeg_error_has_timestamp_discontinuity "$ffmpeg_err"; then
                log_warn "Encode retry: timestamp/PTS anomaly detected; retrying with genpts"
                rm -f "$output"
                encode_timestamp_fix=true

                if run_encode_attempt "$input" "$output" "$video_stream_idx" "$ffmpeg_err" "$encode_include_attachments" "$encode_metadata_mode" "$encode_include_subtitles" "$encode_muxing_queue_size" "$encode_timestamp_fix"; then
                    result=0
                else
                    result=$?
                fi
            fi
        fi
    fi
    
    local elapsed=$(( $(date +%s) - start ))
    
    if [[ $result -eq 0 && -f "$output" ]]; then
        local in_sz out_sz ratio
        in_sz=$(stat -c%s "$input" 2>/dev/null) || in_sz=0
        out_sz=$(stat -c%s "$output" 2>/dev/null) || out_sz=0
        [[ $in_sz -gt 0 ]] && ratio=$((out_sz * 100 / in_sz)) || ratio=0
        rm -f "$ffmpeg_err"
        log_success "Done in ${elapsed}s (${ratio}%)"
        return 0
    else
        log_error "Failed!"
        if [[ -s "$ffmpeg_err" ]]; then
            log_error "Last ffmpeg output:"
            tail -n 20 "$ffmpeg_err" | sed 's/^/  /'
        fi
        rm -f "$ffmpeg_err"
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
    local profile_label="$CPU_HEVC_PROFILE"
    [[ "$ENCODER_MODE" == "vaapi" ]] && profile_label="$VAAPI_PROFILE"
    log_info "Mode: $ENCODER_MODE (HEVC ${profile_label}), QP/CRF: $([[ $ENCODER_MODE == vaapi ]] && echo $VAAPI_QP || echo $CPU_CRF)"
    if [[ "$AUTO_CPU_MODE_FOR_MP4_SAFETY" == true ]]; then
        log_warn "MP4 safety: VAAPI mode auto-switched to CPU to avoid decoder corruption (override with --allow-unsafe-vaapi-mp4)"
    fi
    [[ "$FORCE_HEVC_10BIT" == true ]] && log_warn "HEVC profile override: forcing 10-bit main10 output (--hevc-10bit)"
    log_info "Container: ${OUTPUT_CONTAINER^^}"
    log_info "Audio: All tracks -> ${AUDIO_CHANNELS}ch AAC ${AUDIO_BITRATE} (strict AAC, no source-audio fallback) | Keyframes: source/default cadence"
    if [[ "$KEEP_SUBTITLES" == true ]]; then
        if output_container_is_mp4; then
            log_info "Subtitles: convert to mov_text when compatible (auto-retry without subtitles on incompatible formats)"
        else
            log_info "Subtitles: Copy all subtitle streams (ASS and others preserved)"
        fi
    fi
    if [[ "$KEEP_ATTACHMENTS" == true ]]; then
        if output_container_is_mp4; then
            log_info "Attachments: disabled for MP4 container compatibility"
        else
            log_info "Attachments: Copy font/image attachments"
        fi
    fi
    [[ "$CLEAN_METADATA" == true ]] && log_info "Metadata: clean container metadata and chapters"
    [[ "$CLEAN_METADATA" == false ]] && log_info "Metadata: preserve source container metadata and chapters"
    [[ "$SHOW_FFMPEG_FPS" == true ]] && log_info "FFmpeg progress: live FPS/speed enabled"
    [[ "$SHOW_FILE_STATS" == true ]] && log_info "File stats: source video resolution/bitrate section enabled"
    if [[ "$SKIP_HEVC" == true ]]; then
        log_info "HEVC files: remux (copy video, encode audio)"
        if output_container_is_mp4; then
            log_warn "HEVC copy to MP4 can decode poorly on some Edge systems. Use --no-skip-hevc for safer re-encode."
        fi
    else
        log_info "HEVC files: re-encode video for compatibility (skip-hevc disabled)"
        [[ "$AUTO_EDGE_SAFE_HEVC_REENCODE" == true ]] && log_info "HEVC policy: auto-disabled skip-hevc for MP4 edge safety (override with --skip-hevc)"
    fi
    [[ "$STRICT_MODE" == true ]] && log_info "Retry policy: strict mode enabled (automatic retries disabled)"
    [[ "$CLEAN_TIMESTAMPS" == true ]] && log_info "Timestamps: proactive regeneration enabled by default (genpts + avoid_negative_ts)"
    [[ "$MATCH_AUDIO_LAYOUT" == true ]] && log_info "Audio render compatibility: normalize all audio streams to stereo with stable resampling"
    echo
    
    # Main per-file pipeline:
    # 1) classify filename and compute output path
    # 2) optionally remux HEVC sources in skip-hevc mode
    # 3) otherwise transcode with encode_file
    for f in "${files[@]}"; do
        ((current++))
        
        log_info "[$current/$total] $(basename "$f")"
        
        # Parse filename first to get output path
        parse_filename "$(basename "$f")" "$(basename "$(dirname "$f")")"
        local out
        out=$(get_output_path)
        local video_idx video_codec video_resolution video_bitrate_label
        video_idx=$(get_primary_video_stream_index "$f")
        video_codec=$(get_primary_video_codec "$f")
        [[ -z "$video_codec" ]] && video_codec=$(get_codec "$f")
        video_resolution=$(get_primary_video_resolution "$f")
        video_bitrate_label=$(get_primary_video_bitrate_label "$f")
        log_debug "Primary video stream: index=${video_idx:-0}, codec=${video_codec:-unknown}"
        if [[ "$SHOW_FILE_STATS" == true ]]; then
            log_info "File Stats:"
            log_info "  Video: ${video_resolution} | ${video_bitrate_label} | codec=${video_codec:-unknown}"
        fi
        
        # Check if already HEVC - copy video, but still encode audio to AAC
        if [[ "$SKIP_HEVC" == true ]]; then
            if [[ "$video_codec" == "hevc" ]]; then
                if [[ "$SKIP_EXISTING" == true && -f "$out" ]]; then
                    log_warn "Skip (exists): $(basename "$out")"
                    ((skipped++))
                    echo
                    continue
                fi
                log_info "Remuxing (HEVC -> copy video, encode AAC): $(basename "$f")"
                log_info "  -> $(basename "$out")"
                mkdir -p "$(dirname "$out")"
                if [[ "$DRY_RUN" == true ]]; then
                    log_success "[DRY] Would remux"
                else
                    local start_rm=$(date +%s)
                    local remux_err remux_ok=false
                    remux_err=$(mktemp)
                    local remux_metadata_mode="strip"
                    local remux_include_subtitles=true
                    local remux_include_attachments=true
                    local remux_muxing_queue_size=4096
                    local remux_timestamp_fix="$CLEAN_TIMESTAMPS"
                    [[ "$CLEAN_METADATA" == false ]] && remux_metadata_mode="keep"

                    # Primary attempt follows the selected metadata mode.
                    if run_remux_attempt "$f" "$out" "${video_idx:-0}" "$remux_metadata_mode" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                        remux_ok=true
                    fi

                    if [[ "$STRICT_MODE" != true ]]; then
                        if [[ "$remux_ok" != true && "$remux_metadata_mode" == "keep" ]]; then
                            log_warn "Remux retry: switching to clean metadata mode"
                            remux_metadata_mode="strip"
                            rm -f "$out"

                            # Fallback to clean metadata mode if preserve mode fails.
                            if run_remux_attempt "$f" "$out" "${video_idx:-0}" "$remux_metadata_mode" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                                remux_ok=true
                            fi
                        fi

                        if [[ "$remux_ok" != true && "$KEEP_ATTACHMENTS" == true ]] && ffmpeg_error_has_attachment_tag_issue "$remux_err"; then
                            log_warn "Remux retry: source attachment tag issue; retrying without attachments"
                            rm -f "$out"
                            remux_include_attachments=false

                            if run_remux_attempt "$f" "$out" "${video_idx:-0}" "$remux_metadata_mode" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                                remux_ok=true
                            fi
                        fi

                        if [[ "$remux_ok" != true && "$KEEP_SUBTITLES" == true && "$remux_include_subtitles" == true ]] && ffmpeg_error_has_subtitle_mux_issue "$remux_err"; then
                            log_warn "Remux retry: subtitle stream mux issue; retrying without subtitles"
                            rm -f "$out"
                            remux_include_subtitles=false

                            if run_remux_attempt "$f" "$out" "${video_idx:-0}" "$remux_metadata_mode" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                                remux_ok=true
                            fi
                        fi

                        if [[ "$remux_ok" != true && "$remux_muxing_queue_size" -lt 16384 ]] && ffmpeg_error_has_mux_queue_overflow "$remux_err"; then
                            log_warn "Remux retry: increasing mux queue size to 16384"
                            rm -f "$out"
                            remux_muxing_queue_size=16384

                            if run_remux_attempt "$f" "$out" "${video_idx:-0}" "$remux_metadata_mode" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                                remux_ok=true
                            fi
                        fi

                        if [[ "$remux_ok" != true && "$remux_timestamp_fix" != true ]] && ffmpeg_error_has_timestamp_discontinuity "$remux_err"; then
                            log_warn "Remux retry: timestamp/PTS anomaly detected; retrying with genpts"
                            rm -f "$out"
                            remux_timestamp_fix=true

                            if run_remux_attempt "$f" "$out" "${video_idx:-0}" "$remux_metadata_mode" "$remux_err" "$remux_include_attachments" "$remux_include_subtitles" "$remux_muxing_queue_size" "$remux_timestamp_fix"; then
                                remux_ok=true
                            fi
                        fi
                    fi

                    if [[ "$remux_ok" != true ]]; then
                        log_error "Remux failed"
                        if [[ -s "$remux_err" ]]; then
                            log_error "Last ffmpeg output:"
                            tail -n 20 "$remux_err" | sed 's/^/  /'
                        fi
                        rm -f "$remux_err"
                        rm -f "$out"
                        ((failed++))
                        echo
                        continue
                    fi
                    rm -f "$remux_err"
                    
                    local elapsed_rm=$(( $(date +%s) - start_rm ))
                    local in_sz out_sz ratio
                    in_sz=$(stat -c%s "$f" 2>/dev/null) || in_sz=0
                    out_sz=$(stat -c%s "$out" 2>/dev/null) || out_sz=0
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
        
        if encode_file "$f" "$out" "${video_idx:-0}"; then
            ((encoded++))
        else
            ((failed++))
        fi
        echo
    done
    
    log_info "=============================="
    log_info "Done: $encoded encoded, $skipped skipped, $failed failed"

    return 0
}

# Lightweight environment diagnostics for quick troubleshooting.
run_check() {
    log_info "=== System Check ==="
    
    if command -v ffmpeg &>/dev/null; then
        local ffmpeg_version
        ffmpeg_version=$(ffmpeg -version 2>/dev/null | sed -n '1p')
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
            log_success "VAAPI works"
        else
            log_error "VAAPI failed"
        fi
    fi
    
    log_info "Testing CPU x265..."
    if ffmpeg -hide_banner -nostdin -f lavfi -i color=black:s=256x256:d=0.1 -c:v libx265 -f null - 2>/dev/null; then
        log_success "CPU x265 works"
    else
        log_error "CPU x265 failed"
    fi
}

# Entrypoint
main() {
    init_colors
    parse_args "$@"
    init_colors
    print_banner
    
    if [[ "$CHECK_ONLY" == true ]]; then
        run_check
        exit 0
    fi
    
    [[ ! -d "$INPUT_DIR" ]] && { log_error "Input not found: $INPUT_DIR"; exit 1; }
    mkdir -p "$OUTPUT_DIR"
    
    log_info "=== Muxmaster v${SCRIPT_VERSION} ==="
    log_info "In:  $INPUT_DIR"
    log_info "Out: $OUTPUT_DIR"
    [[ "$DRY_RUN" == true ]] && log_warn "DRY RUN"
    echo
    
    check_deps
    process_files
}

main "$@"

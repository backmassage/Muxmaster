#!/usr/bin/env python3
"""
Jellyfin library crawler and ffprobe CSV reporter.

Recursively scans a media directory, prints a concise per-file summary to
terminal, and writes detailed stream metadata to a CSV file.
"""

from __future__ import annotations

import argparse
import csv
import datetime as dt
import json
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, Iterable, List, Optional, Sequence, Tuple


DEFAULT_EXTENSIONS = {
    ".mkv",
    ".mp4",
    ".avi",
    ".m4v",
    ".mov",
    ".wmv",
    ".flv",
    ".webm",
    ".ts",
    ".m2ts",
    ".mpg",
    ".mpeg",
    ".vob",
    ".ogv",
}


CSV_FIELDS = [
    "scan_timestamp_utc",
    "library_root",
    "relative_path",
    "absolute_path",
    "file_name",
    "extension",
    "size_bytes",
    "size_human",
    "container_formats",
    "duration_seconds",
    "duration_human",
    "format_bitrate_bps",
    "video_stream_count",
    "audio_stream_count",
    "subtitle_stream_count",
    "attachment_stream_count",
    "data_stream_count",
    "video_codecs",
    "video_profiles",
    "video_resolutions",
    "video_pix_fmts",
    "video_color_primaries",
    "video_color_transfer",
    "video_color_space",
    "video_is_hdr",
    "audio_codecs",
    "audio_channels",
    "audio_channel_layouts",
    "audio_sample_rates",
    "audio_bitrates",
    "audio_languages",
    "audio_titles",
    "subtitle_codecs",
    "subtitle_languages",
    "subtitle_titles",
    "subtitle_default_count",
    "subtitle_forced_count",
    "ffprobe_ok",
    "ffprobe_error",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Recursively crawl a Jellyfin media directory and export ffprobe "
            "metadata to CSV."
        )
    )
    parser.add_argument(
        "media_dir",
        help="Root media directory to scan recursively.",
    )
    parser.add_argument(
        "-o",
        "--csv",
        default="",
        help=(
            "Output CSV path. Defaults to "
            "./jellyfin_library_report_<timestamp>.csv"
        ),
    )
    parser.add_argument(
        "--ffprobe-bin",
        default="ffprobe",
        help="ffprobe binary path/name (default: ffprobe).",
    )
    parser.add_argument(
        "--include-ext",
        action="append",
        default=[],
        help=(
            "Additional extension to include (repeatable), for example: "
            "--include-ext .m2v"
        ),
    )
    parser.add_argument(
        "--quiet",
        action="store_true",
        help="Disable per-file terminal lines (still prints final summary).",
    )
    return parser.parse_args()


def ordered_unique(values: Iterable[Any]) -> List[str]:
    seen = set()
    out: List[str] = []
    for value in values:
        if value is None:
            continue
        text = str(value).strip()
        if not text or text in seen:
            continue
        seen.add(text)
        out.append(text)
    return out


def join_values(values: Iterable[Any]) -> str:
    return ";".join(ordered_unique(values))


def human_size(num_bytes: int) -> str:
    units = ["B", "KB", "MB", "GB", "TB"]
    value = float(max(num_bytes, 0))
    for unit in units:
        if value < 1024.0 or unit == units[-1]:
            if unit == "B":
                return f"{int(value)}{unit}"
            return f"{value:.2f}{unit}"
        value /= 1024.0
    return f"{num_bytes}B"


def format_duration(seconds: Optional[float]) -> str:
    if seconds is None or seconds < 0:
        return "unknown"
    whole = int(seconds)
    hours, rem = divmod(whole, 3600)
    minutes, secs = divmod(rem, 60)
    return f"{hours:02d}:{minutes:02d}:{secs:02d}"


def to_int(value: Any, fallback: int = 0) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return fallback


def to_float(value: Any) -> Optional[float]:
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def compact_list(text: str, fallback: str = "-") -> str:
    items = [x for x in text.split(";") if x]
    if not items:
        return fallback
    if len(items) == 1:
        return items[0]
    return f"{items[0]}+{len(items) - 1}"


def discover_media_files(root: Path, extensions: Sequence[str]) -> List[Path]:
    ext_set = {ext.lower() for ext in extensions}
    files: List[Path] = []
    for path in sorted(root.rglob("*")):
        if path.is_file() and path.suffix.lower() in ext_set:
            files.append(path)
    return files


def check_ffprobe_available(ffprobe_bin: str) -> Tuple[bool, str]:
    try:
        proc = subprocess.run(
            [ffprobe_bin, "-version"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            check=False,
        )
    except FileNotFoundError:
        return False, f"ffprobe binary not found: {ffprobe_bin}"

    if proc.returncode != 0:
        detail = proc.stderr.strip() or proc.stdout.strip() or "unknown error"
        return False, f"ffprobe is unavailable: {detail}"

    return True, ""


def run_ffprobe(ffprobe_bin: str, media_path: Path) -> Tuple[Optional[Dict[str, Any]], str]:
    cmd = [
        ffprobe_bin,
        "-v",
        "error",
        "-show_entries",
        (
            "format=format_name,duration,bit_rate,size:"
            "stream=index,codec_type,codec_name,profile,width,height,pix_fmt,"
            "bit_rate,channels,channel_layout,sample_rate,color_space,"
            "color_transfer,color_primaries:"
            "stream_tags=language,title:"
            "stream_disposition=default,forced"
        ),
        "-of",
        "json",
        str(media_path),
    ]

    proc = subprocess.run(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )

    if proc.returncode != 0:
        err = proc.stderr.strip() or "ffprobe exited with non-zero status"
        first_line = err.splitlines()[0] if err else "ffprobe error"
        return None, first_line

    try:
        payload = json.loads(proc.stdout or "{}")
    except json.JSONDecodeError as exc:
        return None, f"invalid ffprobe JSON: {exc}"

    return payload, ""


def build_row(
    scan_iso: str,
    root: Path,
    media_path: Path,
    ffprobe_data: Optional[Dict[str, Any]],
    ffprobe_error: str,
) -> Dict[str, str]:
    relative = str(media_path.relative_to(root))
    absolute = str(media_path.resolve())
    size_bytes = media_path.stat().st_size

    row: Dict[str, str] = {field: "" for field in CSV_FIELDS}
    row.update(
        {
            "scan_timestamp_utc": scan_iso,
            "library_root": str(root),
            "relative_path": relative,
            "absolute_path": absolute,
            "file_name": media_path.name,
            "extension": media_path.suffix.lower(),
            "size_bytes": str(size_bytes),
            "size_human": human_size(size_bytes),
            "ffprobe_ok": "false",
            "ffprobe_error": ffprobe_error,
        }
    )

    if not ffprobe_data:
        return row

    streams = ffprobe_data.get("streams", []) or []
    format_info = ffprobe_data.get("format", {}) or {}

    video_streams = [s for s in streams if s.get("codec_type") == "video"]
    audio_streams = [s for s in streams if s.get("codec_type") == "audio"]
    subtitle_streams = [s for s in streams if s.get("codec_type") == "subtitle"]
    attachment_streams = [s for s in streams if s.get("codec_type") == "attachment"]
    data_streams = [s for s in streams if s.get("codec_type") == "data"]

    container_formats = join_values((format_info.get("format_name") or "").split(","))
    duration_seconds = to_float(format_info.get("duration"))
    format_bitrate = to_int(format_info.get("bit_rate"), fallback=0)

    video_resolutions = []
    for stream in video_streams:
        width = stream.get("width")
        height = stream.get("height")
        if width and height:
            video_resolutions.append(f"{width}x{height}")

    audio_languages = []
    audio_titles = []
    audio_channels = []
    audio_layouts = []
    audio_sample_rates = []
    audio_bitrates = []
    for stream in audio_streams:
        tags = stream.get("tags", {}) or {}
        audio_languages.append(tags.get("language", "und"))
        audio_titles.append(tags.get("title", ""))
        audio_channels.append(stream.get("channels"))
        audio_layouts.append(stream.get("channel_layout"))
        audio_sample_rates.append(stream.get("sample_rate"))
        audio_bitrates.append(stream.get("bit_rate"))

    subtitle_languages = []
    subtitle_titles = []
    subtitle_default_count = 0
    subtitle_forced_count = 0
    for stream in subtitle_streams:
        tags = stream.get("tags", {}) or {}
        subtitle_languages.append(tags.get("language", "und"))
        subtitle_titles.append(tags.get("title", ""))
        disposition = stream.get("disposition", {}) or {}
        subtitle_default_count += 1 if to_int(disposition.get("default"), 0) == 1 else 0
        subtitle_forced_count += 1 if to_int(disposition.get("forced"), 0) == 1 else 0

    hdr_detected = False
    for stream in video_streams:
        color_transfer = str(stream.get("color_transfer", "")).lower()
        color_primaries = str(stream.get("color_primaries", "")).lower()
        if color_transfer in {"smpte2084", "arib-std-b67"} or color_primaries == "bt2020":
            hdr_detected = True
            break

    row.update(
        {
            "container_formats": container_formats,
            "duration_seconds": f"{duration_seconds:.3f}" if duration_seconds is not None else "",
            "duration_human": format_duration(duration_seconds),
            "format_bitrate_bps": str(format_bitrate) if format_bitrate > 0 else "",
            "video_stream_count": str(len(video_streams)),
            "audio_stream_count": str(len(audio_streams)),
            "subtitle_stream_count": str(len(subtitle_streams)),
            "attachment_stream_count": str(len(attachment_streams)),
            "data_stream_count": str(len(data_streams)),
            "video_codecs": join_values(s.get("codec_name") for s in video_streams),
            "video_profiles": join_values(s.get("profile") for s in video_streams),
            "video_resolutions": join_values(video_resolutions),
            "video_pix_fmts": join_values(s.get("pix_fmt") for s in video_streams),
            "video_color_primaries": join_values(
                s.get("color_primaries") for s in video_streams
            ),
            "video_color_transfer": join_values(
                s.get("color_transfer") for s in video_streams
            ),
            "video_color_space": join_values(s.get("color_space") for s in video_streams),
            "video_is_hdr": "true" if hdr_detected else "false",
            "audio_codecs": join_values(s.get("codec_name") for s in audio_streams),
            "audio_channels": join_values(audio_channels),
            "audio_channel_layouts": join_values(audio_layouts),
            "audio_sample_rates": join_values(audio_sample_rates),
            "audio_bitrates": join_values(audio_bitrates),
            "audio_languages": join_values(audio_languages),
            "audio_titles": join_values(audio_titles),
            "subtitle_codecs": join_values(s.get("codec_name") for s in subtitle_streams),
            "subtitle_languages": join_values(subtitle_languages),
            "subtitle_titles": join_values(subtitle_titles),
            "subtitle_default_count": str(subtitle_default_count),
            "subtitle_forced_count": str(subtitle_forced_count),
            "ffprobe_ok": "true",
            "ffprobe_error": "",
        }
    )

    return row


def print_row_summary(index: int, total: int, row: Dict[str, str]) -> None:
    rel = row["relative_path"]
    if row["ffprobe_ok"] != "true":
        print(f"[{index}/{total}] {rel} | ERROR: {row['ffprobe_error']}")
        return

    duration = row["duration_human"] or "unknown"
    v_codec = compact_list(row["video_codecs"], fallback="none")
    v_res = compact_list(row["video_resolutions"], fallback="-")
    hdr = " HDR" if row["video_is_hdr"] == "true" else ""
    a_count = row["audio_stream_count"] or "0"
    a_codecs = compact_list(row["audio_codecs"], fallback="-")
    s_count = row["subtitle_stream_count"] or "0"
    s_codecs = compact_list(row["subtitle_codecs"], fallback="-")

    print(
        f"[{index}/{total}] {rel} | {duration} | "
        f"V:{v_codec} {v_res}{hdr} | A:{a_count} ({a_codecs}) | S:{s_count} ({s_codecs})"
    )


def write_csv(csv_path: Path, rows: Sequence[Dict[str, str]]) -> None:
    csv_path.parent.mkdir(parents=True, exist_ok=True)
    with csv_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=CSV_FIELDS)
        writer.writeheader()
        writer.writerows(rows)


def main() -> int:
    args = parse_args()

    media_root = Path(args.media_dir).expanduser().resolve()
    if not media_root.exists() or not media_root.is_dir():
        print(f"ERROR: media directory not found: {media_root}", file=sys.stderr)
        return 2

    ffprobe_ok, ffprobe_msg = check_ffprobe_available(args.ffprobe_bin)
    if not ffprobe_ok:
        print(f"ERROR: {ffprobe_msg}", file=sys.stderr)
        return 2

    extra_extensions = []
    for ext in args.include_ext:
        e = ext.strip().lower()
        if not e:
            continue
        extra_extensions.append(e if e.startswith(".") else f".{e}")

    extensions = sorted(DEFAULT_EXTENSIONS.union(extra_extensions))
    files = discover_media_files(media_root, extensions)

    timestamp = dt.datetime.now(dt.timezone.utc)
    scan_iso = timestamp.isoformat(timespec="seconds")
    if args.csv:
        csv_path = Path(args.csv).expanduser().resolve()
    else:
        csv_name = f"jellyfin_library_report_{timestamp.strftime('%Y%m%d_%H%M%S')}.csv"
        csv_path = (Path.cwd() / csv_name).resolve()

    print(f"Library root: {media_root}")
    print(f"Detected media files: {len(files)}")
    print(f"CSV output: {csv_path}")
    print("")

    rows: List[Dict[str, str]] = []
    ffprobe_failures = 0
    total_size_bytes = 0
    total_duration_seconds = 0.0

    for index, media_path in enumerate(files, start=1):
        ffprobe_data, ffprobe_error = run_ffprobe(args.ffprobe_bin, media_path)
        row = build_row(scan_iso, media_root, media_path, ffprobe_data, ffprobe_error)
        rows.append(row)

        total_size_bytes += to_int(row.get("size_bytes"), 0)
        duration = to_float(row.get("duration_seconds"))
        if duration is not None:
            total_duration_seconds += duration
        if row.get("ffprobe_ok") != "true":
            ffprobe_failures += 1

        if not args.quiet:
            print_row_summary(index, len(files), row)

    write_csv(csv_path, rows)

    print("")
    print("Scan complete.")
    print(f"  Files scanned: {len(files)}")
    print(f"  ffprobe errors: {ffprobe_failures}")
    print(f"  Total size: {human_size(total_size_bytes)}")
    print(f"  Total duration: {format_duration(total_duration_seconds)}")
    print(f"  CSV written: {csv_path}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())

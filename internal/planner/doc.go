// Package planner decides per-file action (encode, remux, or skip) and
// builds a FilePlan that the ffmpeg package consumes.
//
// Planned implementation:
//
// Types:
//   - FilePlan (Action, VideoCodec, AudioPlan, SubtitlePlan, FilterChain, etc.)
//   - AudioStreamPlan, SubtitlePlan, AttachmentPlan
//
// Functions:
//   - BuildPlan(Config, ProbeResult) → FilePlan
//     Decision matrix with edge-safe HEVC check.
//   - SmartQuality(Config, ProbeResult) → int
//     Per-file QP/CRF from resolution/bitrate with configurable bias
//     and retry step.
//   - EstimateBitrate(ProbeResult) → int
//     Ratio tables and codec/resolution biases for quality retry
//     when output > 105% source.
//   - BuildVideoFilter(Config, ProbeResult) → string
//     Deinterlace (yadif), HDR tonemap, hwupload for VAAPI.
//   - AudioPlan(Config, ProbeResult) → []AudioStreamPlan
//     Per-stream: copy AAC, transcode to AAC, filter chains,
//     MATCH_AUDIO_LAYOUT support.
//   - SubtitlePlan(Config, ProbeResult) → SubtitlePlan
//     mov_text for MP4, skip bitmap subs in MP4.
//   - DispositionFlags(FilePlan) → []string
//     Default video + first audio; -disposition opts for ffmpeg.
//
// When implementing, split into planner.go, quality.go, estimation.go,
// filter.go, audio.go, subtitle.go, disposition.go.
package planner

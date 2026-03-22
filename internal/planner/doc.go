// Package planner decides per-file action (encode, remux, or skip) and
// builds a FilePlan that the ffmpeg package consumes.
//
// Files:
//   - types.go:       FilePlan, Action, AudioPlan, AudioStreamPlan, SubtitlePlan, AttachmentPlan
//   - planner.go:     BuildPlan entry point — wires all sub-plans into a FilePlan
//   - quality.go:     SmartQuality — per-file QP/CRF from resolution/bitrate curves
//   - estimation.go:  EstimateBitrate — ratio-based output prediction with bias adjustments
//   - tables.go:      Lookup tables for all quality curves, ratio estimation, and biases
//   - filter.go:      BuildVideoFilter, BuildColorOpts, BuildHDR10Meta — filters, color, HDR10 metadata passthrough
//   - audio.go:       BuildAudioPlan — per-stream strategy with MATCH_AUDIO_LAYOUT filters
//   - subtitle.go:    BuildSubtitlePlan, BuildAttachmentPlan
//   - disposition.go: BuildDispositions — default video + first audio stream flags
package planner

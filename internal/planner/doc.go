// Package planner decides per-file action (encode, remux, or skip) and
// builds a FilePlan that the ffmpeg package consumes.
//
// Files:
//   - types.go:       FilePlan, Action, AudioPlan, AudioStreamPlan, SubtitlePlan, AttachmentPlan
//   - planner.go:     BuildPlan entry point — wires all sub-plans into a FilePlan
//   - quality.go:     SmartQuality — per-file QP/CRF from resolution/bitrate curves
//   - estimation.go:  EstimateBitrate — ratio tables with codec/resolution/bitrate biases
//   - filter.go:      BuildVideoFilter, BuildColorOpts — deinterlace, HDR tonemap, VAAPI hwupload
//   - audio.go:       BuildAudioPlan — per-stream strategy with MATCH_AUDIO_LAYOUT filters
//   - subtitle.go:    BuildSubtitlePlan, BuildAttachmentPlan
//   - disposition.go: BuildDispositions — default video + first audio stream flags
package planner

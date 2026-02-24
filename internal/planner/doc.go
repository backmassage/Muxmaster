// Package planner decides per-file action (encode, remux, or skip) and
// builds a FilePlan that the ffmpeg package consumes.
//
// Implemented:
//   - FilePlan, Action, AudioPlan, AudioStreamPlan, SubtitlePlan, AttachmentPlan (types.go)
//   - BuildPlan: decision matrix with HEVC edge-safe check, basic video/audio/subtitle/
//     disposition plans (planner.go)
//
// Planned additions (split into dedicated files when implementing):
//   - SmartQuality: per-file QP/CRF from resolution/bitrate with configurable bias (quality.go)
//   - EstimateBitrate: ratio tables and codec/resolution biases (estimation.go)
//   - BuildVideoFilter: HDR tonemap filter chains (filter.go)
//   - Advanced audio layout normalization (audio.go)
package planner

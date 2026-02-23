// Package pipeline orchestrates discovery, per-file processing, and summary.
package pipeline

// TODO: Implement batch runner: discover -> year index -> for each file validate/probe/plan/execute -> summary.
// Run(ctx, cfg, log) -> RunStats; wires probe, naming, planner, ffmpeg, display.

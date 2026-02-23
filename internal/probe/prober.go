package probe

// TODO: Implement Prober interface and ffprobe implementation.
// Single JSON call per file: -print_format json -show_format -show_streams.
// Returns *ProbeResult; all downstream logic consumes typed structs.

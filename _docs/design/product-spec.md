# Muxmaster – Design Document
Version: 2.0  
Target Platform: Arch Linux  
Language: Go  
Execution Model: CLI (single-run), systemd-compatible  

> **Status:** Aspirational v2.0 design. The core pipeline is implemented (v2.1.0). Post-MVP features — subcommand CLI, persistent state store, atomic output, config file, JSON logging — are deferred. See [foundation-plan.md](foundation-plan.md) §3 for the full scope boundary.

---

# 1. Purpose

Muxmaster is a deterministic, idempotent CLI application that:

- Scans media directories  
- Probes media files using ffprobe  
- Evaluates them against a defined policy  
- Transcodes or remuxes when necessary using ffmpeg  
- Verifies outputs before replacing originals  
- Logs structured, machine-parseable results  
- Maintains persistent state for resumability  

Muxmaster v2.0 replaces a complex Bash (.sh) script with a structured, strongly typed, maintainable Go implementation.

It is an orchestration layer over ffmpeg and ffprobe, not a codec implementation.

---

# 2. Non-Goals

- Media playback  
- Distributed transcoding cluster  
- Custom codec implementation  
- Real-time streaming  
- Real-time monitoring daemon (initially CLI only)  

---

# 3. High-Level Architecture

```
CLI Layer
    ↓
Scanner
    ↓
MediaFile (Domain Object)
    ├── Probe()
    ├── Evaluate()
    ├── Transcode()
    ├── Verify()
    ├── Persist()
    ↓
State Store
```

The `MediaFile` object encapsulates lifecycle state and behavior.  
All file-specific logic is owned by this object.

---

# 4. Execution Model

Mode: Single-run CLI.

Workflow:

```
main()
  ├── LoadConfig()
  ├── InitLogger()
  ├── PreflightCheck()
  ├── DiscoverFiles()
  ├── For each file:
  │       Create MediaFile
  │       Probe()
  │       Evaluate()
  │       Transcode() if required
  │       Verify()
  │       Persist()
  ├── Summarize()
  └── ExitCode()
```

### Systemd Compatibility

- Logs to stdout/stderr  
- Structured JSON optional  
- Meaningful exit codes  
- Restart-safe and idempotent  
- Suitable for systemd timers  

---

# 5. Core Domain Model

## 5.1 MediaFile

Primary domain object representing one media file and its lifecycle.

### Fields

```
Path
ProbeInfo
Plan
ExecutionResult
State
```

### Lifecycle States

```
Discovered
Probed
Planned
Transcoded
Verified
Skipped
Failed
```

### Methods

```
Probe(prober)
Evaluate(policy)
Transcode(encoder)
Verify(verifier)
Persist(store)
```

The object owns all state transitions.  
External callers do not manipulate internal fields directly.

---

## 5.2 ProbeInfo (Aligned with ffprobe JSON Spec)

ProbeInfo models structured output from:

```
ffprobe -v quiet -print_format json -show_format -show_streams
```

### Structure

```
ProbeInfo
 ├── Format
 ├── Streams
 ├── VideoStreams
 ├── AudioStreams
 ├── SubtitleStreams
```

Raw JSON is parsed once and normalized into typed structures.

### FormatInfo

Derived from the `format` section.

Fields:

- Filename  
- NbStreams  
- FormatName  
- FormatLongName  
- Duration (float64)  
- Size (int64)  
- BitRate (int64)  
- Tags (map[string]string)  

String numbers from ffprobe are converted into typed values.

### StreamInfo (Common Fields)

- Index  
- CodecName  
- CodecLongName  
- CodecType (video, audio, subtitle, data)  
- CodecTagString  
- CodecTag  
- BitRate  
- Duration  
- Profile  
- Tags  

### VideoStreamInfo

Extends StreamInfo with:

- Width  
- Height  
- PixFmt  
- AvgFrameRate  
- RFrameRate  
- FieldOrder  
- ColorRange  
- ColorSpace  
- ColorTransfer  
- ColorPrimaries  
- Disposition  

### AudioStreamInfo

Extends StreamInfo with:

- SampleRate  
- Channels  
- ChannelLayout  
- BitsPerSample  
- Disposition  
- Language (from tags)  

### SubtitleStreamInfo

Extends StreamInfo with:

- Language  
- Disposition  
- CodecName (ass, srt, pgssub, etc.)  

### Normalization Rules

After parsing:

- Numeric strings converted to proper types  
- Missing values defaulted safely  
- Streams categorized by type  
- Invalid or corrupt streams trigger MediaError  

ProbeInfo becomes immutable after creation.

---

## 5.3 Plan

Represents the evaluation decision.

Fields:

- ShouldTranscode (bool)  
- TargetVideoCodec  
- TargetAudioCodec  
- TargetContainer  
- Reason  

Produced by Policy.

---

## 5.4 ExecutionResult

Represents outcome of transcode/remux.

Fields:

- OutputPath  
- Duration  
- SizeBefore  
- SizeAfter  
- Success  
- ErrorType  

---

# 6. Interfaces (Dependency Inversion)

All external behavior abstracted behind interfaces.

## Prober

```
Probe(path string) (*ProbeInfo, error)
```

Implementation: ffprobe wrapper.

## Encoder

```
Encode(input, output string, plan Plan) error
```

Implementation: ffmpeg wrapper.

## Policy

```
Evaluate(info ProbeInfo) Plan
```

## StateStore

```
Load(path string) (*Record, error)
Save(record Record) error
```

Implementation: SQLite or BoltDB.

---

# 7. Error Model

Error kinds:

```
ConfigError
EnvironmentError
MediaError
TransientError
PolicySkip
VerificationError
```

Custom error:

```
AppError {
    Kind
    Err
}
```

Rules:

- Media errors do not stop batch  
- Environment errors may abort  
- Transient errors may retry  
- All failures logged structurally  

---

# 8. Logging Design

Supports:

- Text mode (default)  
- JSON mode (--json)  

Log levels:

```
DEBUG
INFO
WARN
ERROR
```

Each log entry includes:

- file  
- stage  
- state  
- duration  
- error_kind  

---

## 8.1 CLI Logging Flags

```
--log-level=debug|info|warn|error
--json
```

### Probe Logging Behavior

INFO:

```
Probing file: /media/anime/file.mkv
Detected: hevc video, aac audio, 3 streams
```

DEBUG includes:

- Full ProbeInfo dump  
- Stream-level metadata  
- ffprobe execution time  
- stderr output  

Failure logs include:

- Exit code  
- stderr  
- Error classification  
- Executed command  

Logging principles:

- No silent failures  
- Log before and after state transitions  
- Always include file path  
- Journal-friendly structure  

---

# 9. Safety and Hardening

## Preflight Checks

- ffmpeg exists  
- ffprobe exists  
- Output writable  
- Disk space sufficient  

Fail fast if unmet.

## Atomic Output Strategy

1. Write to temp file  
2. Verify output  
3. Atomic rename  
4. Persist state  

Original never overwritten before verification.

## Crash Safety

- Persist after each file  
- Skip completed files  
- Retry failed optionally  
- Fully idempotent  

---

# 10. Configuration Model

Location:

```
~/.config/muxmaster/config.yaml
```

Priority:

```
CLI flags > env vars > config file > defaults
```

Config defines:

- Target codecs  
- Bitrate rules  
- Container format  
- Logging mode  
- Retry policy  
- Temp directory  

---

# 11. CLI Interface

Commands:

```
muxmaster scan
muxmaster plan
muxmaster run
muxmaster verify
muxmaster stats
```

Flags:

```
--config
--json
--dry-run
--verbose
--retry-failed
```

Exit Codes:

```
0 = success
1 = fatal error
2 = partial failure
```

---

# 12. Persistence Strategy

Stored per file:

- Path  
- Hash or size+mtime  
- Last status  
- Output path  
- Last error type  

Enables:

- Resume support  
- Skip unchanged files  
- Failure tracking  

---

# 13. Deployment Model (Arch Linux)

- Static Go binary  
- No runtime dependencies  
- Run as unprivileged user  

Optional systemd hardening:

```
ProtectSystem=full
NoNewPrivileges=true
PrivateTmp=true
```

---

# 14. Extensibility

Future expansion:

- Web UI  
- REST API  
- Parallel workers  
- Distributed encoding  
- Metrics endpoint  

Enabled by:

- Isolated domain model  
- Interface-driven design  
- Structured logging  
- Externalized state  

---

# 15. Rationale for Go

Go selected because:

- Static single binary  
- Strong process control  
- Clean interface abstractions  
- Explicit error handling  
- Excellent Linux integration  
- Low operational overhead  

Muxmaster v2.0 replaces fragile shell logic with typed state, structured logging, hardened error handling, and deterministic execution.

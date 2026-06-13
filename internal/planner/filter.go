// Video filter chain: deinterlace, HDR tonemap, VAAPI hw/sw decode paths.
package planner

import (
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildVideoFilter constructs the comma-joined ffmpeg video filter chain
// for the encode path. When hwDecode is true, frames are already on the
// GPU as VAAPI surfaces — scale_vaapi handles format conversion and
// deinterlace uses the GPU-native filter instead of CPU yadif.
// When hwDecode is false, CPU-side format conversion and hwupload are
// used for the VAAPI path; returns empty for CPU-only encodes with no
// deinterlace or tonemap.
func BuildVideoFilter(cfg *config.Config, pr *probe.ProbeResult, hwDecode bool) string {
	if hwDecode {
		return buildVAAPIHWDecodeFilters(cfg, pr)
	}
	return buildSoftwareDecodeFilters(cfg, pr)
}

// buildVAAPIHWDecodeFilters builds the filter chain when VAAPI hardware
// decode is active. Frames arrive as VAAPI surfaces so no hwupload is
// needed. A scale_vaapi format conversion ensures decoded surfaces match
// the encoder's expected pixel format — H264 8-bit sources produce NV12
// surfaces while the main10 profile needs P010. Without this, GPUs that
// can't do implicit NV12→P010 promotion fail with "No usable encoding
// profile found." The conversion is a no-op when formats already match.
func buildVAAPIHWDecodeFilters(cfg *config.Config, pr *probe.ProbeResult) string {
	var filters []string

	if cfg.Encoder.DeinterlaceAuto && pr.IsInterlaced() {
		filters = append(filters, "deinterlace_vaapi")
	}

	swFormat := cfg.Encoder.VaapiSwFormat
	if swFormat == "" {
		swFormat = "p010"
	}
	filters = append(filters, "scale_vaapi=format="+swFormat)

	return strings.Join(filters, ",")
}

// buildSoftwareDecodeFilters builds the filter chain for the software-decode
// path (CPU decode, optional CPU filters, then hwupload for VAAPI encode).
func buildSoftwareDecodeFilters(cfg *config.Config, pr *probe.ProbeResult) string {
	var filters []string

	if cfg.Encoder.DeinterlaceAuto && pr.IsInterlaced() {
		filters = append(filters, "yadif=mode=send_frame:parity=auto:deint=interlaced")
	}

	// Content-aware prefilter (denoise/deband). Placed after deinterlace
	// (denoise must follow deinterlace) and before tonemap/scale/hwupload
	// (deband before scaling). This single function feeds both the VAAPI-sw
	// and CPU chains, so CPU mode gets the prefilter for free.
	if pre := TunePrefilter(cfg); pre != "" {
		filters = append(filters, pre)
	}

	if pr.HDRType() == "hdr10" && cfg.Encoder.HandleHDR == config.HDRTonemap {
		if cfg.Encoder.Mode == config.EncoderVAAPI {
			swFormat := cfg.Encoder.VaapiSwFormat
			if swFormat == "" {
				swFormat = "nv12"
			}
			filters = append(filters, vaapiTonemapChain(swFormat))
		} else {
			filters = append(filters, cpuTonemapChain)
		}
	}

	if cfg.Encoder.Mode == config.EncoderVAAPI {
		tonemapped := pr.HDRType() == "hdr10" && cfg.Encoder.HandleHDR == config.HDRTonemap
		if !tonemapped {
			swFormat := cfg.Encoder.VaapiSwFormat
			if swFormat == "" {
				swFormat = "p010"
			}
			filters = append(filters, "format="+swFormat)
		}
		filters = append(filters, "hwupload")
	}

	return strings.Join(filters, ",")
}

// Denoise/deband strengths for the --tune prefilters (Change 3). Parameterized
// as named constants so the sweep-selected hqdn3d/nlmeans strings drop straight
// in without touching TunePrefilter's logic.
//
//   - filmDenoise is measured strong on VCN (−21% size / −0.15 VMAF, memory) —
//     kept as the default value until the sweep says otherwise.
//   - grainDenoise is an INITIAL ESTIMATE: strong hqdn3d destroys fine detail
//     (dr-lex); the sweep evaluates gentler hqdn3d and an nlmeans variant by
//     quality-per-bit. Re-calibrate before treating as final.
//   - animeDeband: gradfun strength 1.2 is ffmpeg's default; valid minimum 0.51
//     (0.5 errors at graph init). `deband` is the heavier alternative.
const (
	filmDenoise  = "hqdn3d=1.5:1.5:6:6"
	grainDenoise = "hqdn3d=4:4:9:9" // INITIAL ESTIMATE — calibrate via sweep.
	animeDeband  = "gradfun=1.2:16"
)

// TunePrefilter returns the CPU filter string for the active --tune profile,
// or "" for TuneNone. These run on the software-decode path only (CPU filters
// cannot be inserted into a hardware-decoded VAAPI surface chain — ffmpeg
// errors with -22), so an active prefilter forces software decode in VAAPI
// mode (see planner.BuildPlan). Constants are tuned for raw Blu-ray sources;
// re-confirm strengths on real content before treating them as final.
func TunePrefilter(cfg *config.Config) string {
	switch cfg.Encoder.Tune {
	case config.TuneFilm:
		return filmDenoise
	case config.TuneGrain:
		return grainDenoise
	case config.TuneAnime:
		return animeDeband
	default:
		// TuneAuto and TuneNone carry no prefilter. TuneAuto is a pipeline-level
		// sentinel resolved to a concrete profile (via the grain pre-pass +
		// per-series prompt) before BuildPlan runs; if it ever reaches here
		// unresolved it is a safe no-op.
		return ""
	}
}

// cpuTonemapChain is the zscale+tonemap pipeline for converting HDR10 to SDR
// in CPU mode, matching the legacy script exactly.
const cpuTonemapChain = "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709," +
	"tonemap=tonemap=hable:desat=0," +
	"zscale=t=bt709:m=bt709:r=tv,format=yuv420p"

// vaapiTonemapChain returns the zscale+tonemap pipeline for VAAPI mode.
// It is identical to the CPU chain except the final format outputs the
// VAAPI-compatible pixel format (nv12 or p010) instead of yuv420p, avoiding
// a redundant format conversion before hwupload.
func vaapiTonemapChain(swFormat string) string {
	return "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709," +
		"tonemap=tonemap=hable:desat=0," +
		"zscale=t=bt709:m=bt709:r=tv,format=" + swFormat
}

// BuildColorOpts returns the ffmpeg color metadata flags for the encode path.
// Probed color tags are passed through for SDR sources and HDR10 preserve mode
// alike — untagged output forces clients to guess. When the HDR→SDR tonemap
// chain runs, the output is explicitly tagged bt709 (the zscale chain converts
// the pixels but leaves the stream untagged otherwise).
func BuildColorOpts(cfg *config.Config, pr *probe.ProbeResult) []string {
	v := pr.PrimaryVideo
	if v == nil {
		return nil
	}

	if pr.HDRType() == "hdr10" && cfg.Encoder.HandleHDR == config.HDRTonemap {
		// The tonemap chain ends in a 4:4:4→4:2:0 downsample whose chroma
		// siting no longer matches the source, so no chroma tag is emitted.
		return []string{
			"-color_trc", "bt709",
			"-color_primaries", "bt709",
			"-colorspace", "bt709",
		}
	}

	var opts []string
	if tagged(v.ColorTransfer) {
		opts = append(opts, "-color_trc", v.ColorTransfer)
	}
	if tagged(v.ColorPrimaries) {
		opts = append(opts, "-color_primaries", v.ColorPrimaries)
	}
	if tagged(v.ColorSpace) {
		opts = append(opts, "-colorspace", v.ColorSpace)
	}
	if tagged(v.ChromaLocation) {
		opts = append(opts, "-chroma_sample_location", v.ChromaLocation)
	}
	return opts
}

// tagged reports whether a probed color metadata value carries real
// information worth passing through to the output stream.
func tagged(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "", "unknown", "unspecified":
		return false
	default:
		return true
	}
}

// BuildHDR10Meta populates the FilePlan's MasterDisplay and MaxCLL fields
// from the probe result when HDR preserve mode is active and the source
// carries HDR10 static metadata (SMPTE ST.2086 + CTA-861.3).
func BuildHDR10Meta(cfg *config.Config, pr *probe.ProbeResult, plan *FilePlan) {
	if cfg.Encoder.HandleHDR != config.HDRPreserve || pr.HDRType() != "hdr10" {
		return
	}
	v := pr.PrimaryVideo
	if v == nil {
		return
	}
	if v.MasteringDisplay != nil {
		plan.MasterDisplay = v.MasteringDisplay.FFmpegMasterDisplay()
	}
	if v.ContentLightLevel != nil {
		plan.MaxCLL = v.ContentLightLevel.FFmpegMaxCLL()
	}
}

package planner

import (
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildVideoFilter constructs the comma-joined ffmpeg video filter chain
// for the encode path. It handles deinterlacing, HDR tonemapping, and the
// VAAPI format+hwupload suffix. This mirrors the legacy build_video_filter
// function.
//
// Returns an empty string when no filters are needed (CPU mode, progressive,
// SDR or HDR-preserve).
func BuildVideoFilter(cfg *config.Config, pr *probe.ProbeResult) string {
	var filters []string

	// Deinterlace with full yadif parameters matching legacy behavior.
	if cfg.DeinterlaceAuto && pr.IsInterlaced() {
		filters = append(filters, "yadif=mode=send_frame:parity=auto:deint=interlaced")
	}

	// HDR tonemap to SDR.
	if pr.HDRType() == "hdr10" && cfg.HandleHDR == config.HDRTonemap {
		filters = append(filters, cpuTonemapChain)
	}

	// VAAPI requires format conversion and hwupload.
	if cfg.EncoderMode == config.EncoderVAAPI {
		swFormat := cfg.VaapiSwFormat
		if swFormat == "" {
			swFormat = "p010"
		}
		filters = append(filters, "format="+swFormat, "hwupload")
	}

	return strings.Join(filters, ",")
}

// cpuTonemapChain is the zscale+tonemap pipeline for converting HDR10 to SDR
// in CPU mode, matching the legacy script exactly.
const cpuTonemapChain = "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709," +
	"tonemap=tonemap=hable:desat=0," +
	"zscale=t=bt709:m=bt709:r=tv,format=yuv420p"

// BuildColorOpts returns the ffmpeg color metadata flags for HDR preservation
// on the encode path. When HDR is detected and preserve mode is active, the
// source color transfer, primaries, and space are passed through to the output.
func BuildColorOpts(cfg *config.Config, pr *probe.ProbeResult) []string {
	if cfg.HandleHDR != config.HDRPreserve || pr.HDRType() != "hdr10" {
		return nil
	}

	v := pr.PrimaryVideo
	if v == nil {
		return nil
	}

	var opts []string
	if v.ColorTransfer != "" {
		opts = append(opts, "-color_trc", v.ColorTransfer)
	}
	if v.ColorPrimaries != "" {
		opts = append(opts, "-color_primaries", v.ColorPrimaries)
	}
	if v.ColorSpace != "" {
		opts = append(opts, "-colorspace", v.ColorSpace)
	}
	return opts
}

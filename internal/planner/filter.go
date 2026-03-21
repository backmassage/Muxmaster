// Video filter chain: deinterlace, HDR tonemap, VAAPI hw/sw decode paths.
package planner

import (
	"strings"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// BuildVideoFilter constructs the comma-joined ffmpeg video filter chain
// for the encode path. When hwDecode is true, frames are already on the
// GPU as VAAPI surfaces so no format conversion or hwupload is needed;
// deinterlace uses the VAAPI-native filter instead of CPU yadif.
//
// Returns an empty string when no filters are needed.
func BuildVideoFilter(cfg *config.Config, pr *probe.ProbeResult, hwDecode bool) string {
	if hwDecode {
		return buildVAAPIHWDecodeFilters(cfg, pr)
	}
	return buildSoftwareDecodeFilters(cfg, pr)
}

// buildVAAPIHWDecodeFilters builds the filter chain when VAAPI hardware
// decode is active. Frames arrive as VAAPI surfaces — no format conversion
// or hwupload is needed. Deinterlace uses the GPU-native deinterlace_vaapi.
func buildVAAPIHWDecodeFilters(cfg *config.Config, pr *probe.ProbeResult) string {
	if cfg.DeinterlaceAuto && pr.IsInterlaced() {
		return "deinterlace_vaapi"
	}
	return ""
}

// buildSoftwareDecodeFilters builds the filter chain for the software-decode
// path (CPU decode, optional CPU filters, then hwupload for VAAPI encode).
func buildSoftwareDecodeFilters(cfg *config.Config, pr *probe.ProbeResult) string {
	var filters []string

	if cfg.DeinterlaceAuto && pr.IsInterlaced() {
		filters = append(filters, "yadif=mode=send_frame:parity=auto:deint=interlaced")
	}

	if pr.HDRType() == "hdr10" && cfg.HandleHDR == config.HDRTonemap {
		if cfg.EncoderMode == config.EncoderVAAPI {
			swFormat := cfg.VaapiSwFormat
			if swFormat == "" {
				swFormat = "nv12"
			}
			filters = append(filters, vaapiTonemapChain(swFormat))
		} else {
			filters = append(filters, cpuTonemapChain)
		}
	}

	if cfg.EncoderMode == config.EncoderVAAPI {
		tonemapped := pr.HDRType() == "hdr10" && cfg.HandleHDR == config.HDRTonemap
		if !tonemapped {
			swFormat := cfg.VaapiSwFormat
			if swFormat == "" {
				swFormat = "p010"
			}
			filters = append(filters, "format="+swFormat)
		}
		filters = append(filters, "hwupload")
	}

	return strings.Join(filters, ",")
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

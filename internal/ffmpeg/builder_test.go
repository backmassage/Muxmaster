// builder_test.go verifies ffmpeg argument construction from FilePlan + RetryState.
package ffmpeg

import (
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/planner"
)

func cpuCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Encoder.Mode = config.EncoderCPU
	return &cfg
}

func vaapiCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Encoder.Mode = config.EncoderVAAPI
	return &cfg
}

func TestBuild_CPUx265Params_HDR10(t *testing.T) {
	cfg := cpuCfg()
	plan := &planner.FilePlan{
		Action:        planner.ActionEncode,
		VideoCodec:    "libx265",
		InputPath:     "/in/test.mkv",
		OutputPath:    "/out/test.mkv",
		CpuCRF:        18,
		MuxQueueSize:  4096,
		MasterDisplay: "G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,50)",
		MaxCLL:        "1000,400",
	}
	rs := NewRetryState(plan)
	args := Build(cfg, plan, rs)

	var x265Params string
	for i, a := range args {
		if a == "-x265-params" && i+1 < len(args) {
			x265Params = args[i+1]
			break
		}
	}
	if x265Params == "" {
		t.Fatal("missing -x265-params in args")
	}
	if !strings.Contains(x265Params, "master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,50)") {
		t.Errorf("x265-params missing master-display: %q", x265Params)
	}
	if !strings.Contains(x265Params, "max-cll=1000,400") {
		t.Errorf("x265-params missing max-cll: %q", x265Params)
	}
	if !strings.Contains(x265Params, "log-level=error") {
		t.Errorf("x265-params should still contain base params: %q", x265Params)
	}
	if !strings.Contains(x265Params, "hdr10=1:hdr10-opt=1:repeat-headers=1") {
		t.Errorf("x265-params missing HDR10 signaling params: %q", x265Params)
	}
}

func TestBuild_CPUx265Params_SDR(t *testing.T) {
	cfg := cpuCfg()
	plan := &planner.FilePlan{
		Action:       planner.ActionEncode,
		VideoCodec:   "libx265",
		InputPath:    "/in/test.mkv",
		OutputPath:   "/out/test.mkv",
		CpuCRF:       18,
		MuxQueueSize: 4096,
	}
	rs := NewRetryState(plan)
	args := Build(cfg, plan, rs)

	var x265Params string
	for i, a := range args {
		if a == "-x265-params" && i+1 < len(args) {
			x265Params = args[i+1]
			break
		}
	}
	if x265Params != "log-level=error:open-gop=0" {
		t.Errorf("SDR x265-params should be base only, got %q", x265Params)
	}
}

func TestBuild_VAAPI_NoX265Params(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:        planner.ActionEncode,
		VideoCodec:    "hevc_vaapi",
		InputPath:     "/in/test.mkv",
		OutputPath:    "/out/test.mkv",
		VaapiQP:       18,
		MuxQueueSize:  4096,
		MasterDisplay: "G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,50)",
		MaxCLL:        "1000,400",
	}
	rs := NewRetryState(plan)
	args := Build(cfg, plan, rs)

	for _, a := range args {
		if a == "-x265-params" {
			t.Error("VAAPI build should not contain -x265-params")
		}
	}
}

func TestBuild_VAAPI_HWDecodeFallback(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:         planner.ActionEncode,
		VideoCodec:     "hevc_vaapi",
		InputPath:      "/in/test.mkv",
		OutputPath:     "/out/test.mkv",
		VaapiQP:        18,
		MuxQueueSize:   4096,
		HWDecode:       true,
		VideoFilters:   "scale_vaapi=format=p010",
		SWVideoFilters: "format=p010,hwupload",
	}

	// HW decode active: hwaccel flags present, hw filter chain used.
	rs := NewRetryState(plan)
	args := Build(cfg, plan, rs)
	if !containsArg(args, "-hwaccel") {
		t.Error("expected -hwaccel when HW decode is active")
	}
	if got := argValue(args, "-vf"); got != "scale_vaapi=format=p010" {
		t.Errorf("expected hw filter chain, got %q", got)
	}

	// After retry fallback: no hwaccel flags, software chain used.
	rs.HWDecode = false
	args = Build(cfg, plan, rs)
	if containsArg(args, "-hwaccel") {
		t.Error("did not expect -hwaccel after HW decode fallback")
	}
	if got := argValue(args, "-vf"); got != "format=p010,hwupload" {
		t.Errorf("expected software fallback chain, got %q", got)
	}
}

func TestBuild_KeyframeInterval(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:           planner.ActionEncode,
		VideoCodec:       "hevc_vaapi",
		InputPath:        "/in/test.mkv",
		OutputPath:       "/out/test.mkv",
		VaapiQP:          18,
		MuxQueueSize:     4096,
		KeyframeInterval: 120, // 60fps source, 2s GOP
	}
	args := Build(cfg, plan, NewRetryState(plan))
	if got := argValue(args, "-g"); got != "120" {
		t.Errorf("expected per-file GOP 120, got %q", got)
	}

	// Zero falls back to the config default.
	plan.KeyframeInterval = 0
	args = Build(cfg, plan, NewRetryState(plan))
	if got := argValue(args, "-g"); got != "48" {
		t.Errorf("expected config default GOP 48, got %q", got)
	}
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func argValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

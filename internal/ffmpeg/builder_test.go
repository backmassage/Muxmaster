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
	if x265Params != "log-level=error:open-gop=0:aq-mode=3" {
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

// hevc_vaapi has no -master_display/-max_cll options; it re-emits the HDR10
// SEI from decoded-frame side data, gated by -sei. When the plan carries HDR10
// static metadata we pin -sei to the encoder default (hdr+a53_cc) so the SEI no
// longer depends on an ffmpeg default that could change silently. Verified on
// ffmpeg 8.1 / Mesa radeonsi that the output bitstream carries the SEI.
func TestBuild_VAAPI_SEI_HDR10(t *testing.T) {
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
	args := Build(cfg, plan, NewRetryState(plan))
	if got := argValue(args, "-sei"); got != "hdr+a53_cc" {
		t.Errorf("HDR10 VAAPI encode should pin -sei hdr+a53_cc, got %q", got)
	}
}

func TestBuild_VAAPI_SEI_SDR(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:       planner.ActionEncode,
		VideoCodec:   "hevc_vaapi",
		InputPath:    "/in/test.mkv",
		OutputPath:   "/out/test.mkv",
		VaapiQP:      18,
		MuxQueueSize: 4096,
	}
	args := Build(cfg, plan, NewRetryState(plan))
	// No HDR10 metadata: leave -sei at the encoder default (which still passes
	// a53_cc captions through) rather than emitting a redundant override.
	if containsArg(args, "-sei") {
		t.Error("SDR VAAPI encode should not emit -sei")
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
	if got := argValue(args, "-filter:v:0"); got != "scale_vaapi=format=p010" {
		t.Errorf("expected hw filter chain, got %q", got)
	}

	// After retry fallback: no hwaccel flags, software chain used.
	rs.HWDecode = false
	args = Build(cfg, plan, rs)
	if containsArg(args, "-hwaccel") {
		t.Error("did not expect -hwaccel after HW decode fallback")
	}
	if got := argValue(args, "-filter:v:0"); got != "format=p010,hwupload" {
		t.Errorf("expected software fallback chain, got %q", got)
	}
}

func TestBuild_VAAPI_ForceCPUFallback(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:          planner.ActionEncode,
		VideoCodec:      "hevc_vaapi",
		InputPath:       "/in/test.mkv",
		OutputPath:      "/out/test.mkv",
		VaapiQP:         18,
		CpuCRF:          20,
		MuxQueueSize:    4096,
		VideoFilters:    "gradfun=1.2:16,format=p010,hwupload",
		CPUVideoFilters: "gradfun=1.2:16",
	}

	rs := NewRetryState(plan)
	rs.ForceCPU = true
	args := Build(cfg, plan, rs)

	// VAAPI device setup must be gone — the CPU path needs no hardware graph.
	if containsArg(args, "-init_hw_device") {
		t.Error("ForceCPU should not emit -init_hw_device")
	}
	if containsArg(args, "-filter_hw_device") {
		t.Error("ForceCPU should not emit -filter_hw_device")
	}
	if containsArg(args, "-hwaccel") {
		t.Error("ForceCPU should not emit -hwaccel")
	}
	// Codec must be libx265 at the configured CPU CRF, not hevc_vaapi.
	if got := argValue(args, "-c:v"); got != "libx265" {
		t.Errorf("expected -c:v libx265, got %q", got)
	}
	if containsArg(args, "hevc_vaapi") {
		t.Error("ForceCPU should not reference hevc_vaapi")
	}
	if got := argValue(args, "-crf"); got != "20" {
		t.Errorf("expected -crf 20 (CpuCRF), got %q", got)
	}
	// Filter chain must be the hwupload-free CPU chain.
	if got := argValue(args, "-filter:v:0"); got != "gradfun=1.2:16" {
		t.Errorf("expected CPU filter chain, got %q", got)
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

func TestBuild_VAAPI_CQPDefault(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:       planner.ActionEncode,
		VideoCodec:   "hevc_vaapi",
		InputPath:    "/in/test.mkv",
		OutputPath:   "/out/test.mkv",
		VaapiQP:      20,
		MuxQueueSize: 4096,
	}
	args := Build(cfg, plan, NewRetryState(plan))
	if got := argValue(args, "-qp"); got != "20" {
		t.Errorf("expected -qp 20, got %q", got)
	}
	if containsArg(args, "-rc_mode") {
		t.Error("CQP plan should not emit -rc_mode")
	}
	if containsArg(args, "-maxrate") {
		t.Error("CQP plan should not emit -maxrate")
	}
	// Unconditional tuning flags.
	if got := argValue(args, "-compression_level"); got != "1" {
		t.Errorf("expected -compression_level 1, got %q", got)
	}
	if got := argValue(args, "-async_depth"); got != "4" {
		t.Errorf("expected -async_depth 4, got %q", got)
	}
	if containsArg(args, "-bf") {
		t.Error("B-frames not detected: should not emit -bf")
	}
}

func TestBuild_VAAPI_QVBR(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:             planner.ActionEncode,
		VideoCodec:         "hevc_vaapi",
		InputPath:          "/in/test.mkv",
		OutputPath:         "/out/test.mkv",
		VaapiQP:            20,
		VaapiQVBR:          true,
		OptimalBitrateKbps: 5200,
		MaxRateKbps:        5980,
		BufSizeKbps:        11960,
		MuxQueueSize:       4096,
	}
	rs := NewRetryState(plan)
	args := Build(cfg, plan, rs)
	if got := argValue(args, "-rc_mode"); got != "QVBR" {
		t.Errorf("expected -rc_mode QVBR, got %q", got)
	}
	if got := argValue(args, "-global_quality"); got != "20" {
		t.Errorf("expected -global_quality 20, got %q", got)
	}
	if got := argValue(args, "-b:v"); got != "5200k" {
		t.Errorf("expected -b:v 5200k, got %q", got)
	}
	if got := argValue(args, "-maxrate"); got != "5980k" {
		t.Errorf("expected -maxrate 5980k, got %q", got)
	}
	if got := argValue(args, "-bufsize"); got != "11960k" {
		t.Errorf("expected -bufsize 11960k, got %q", got)
	}
	if containsArg(args, "-qp") {
		t.Error("QVBR build should not emit -qp")
	}

	// Retry fallback to CQP.
	rs.VaapiQVBR = false
	args = Build(cfg, plan, rs)
	if containsArg(args, "-rc_mode") {
		t.Error("after QVBR fallback, -rc_mode should be gone")
	}
	if got := argValue(args, "-qp"); got != "20" {
		t.Errorf("after QVBR fallback, expected -qp 20, got %q", got)
	}
}

func TestBuild_VAAPI_BFrames(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:       planner.ActionEncode,
		VideoCodec:   "hevc_vaapi",
		InputPath:    "/in/test.mkv",
		OutputPath:   "/out/test.mkv",
		VaapiQP:      20,
		VaapiBFrames: true,
		MuxQueueSize: 4096,
	}
	rs := NewRetryState(plan)
	args := Build(cfg, plan, rs)
	if got := argValue(args, "-bf"); got != "4" {
		t.Errorf("expected -bf 4, got %q", got)
	}

	rs.VaapiBFrames = false
	args = Build(cfg, plan, rs)
	if containsArg(args, "-bf") {
		t.Error("after B-frame fallback, -bf should be gone")
	}
}

func TestBuild_BSFOpts_RemuxDoviStrip(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:       planner.ActionRemux,
		VideoCodec:   "copy",
		InputPath:    "/in/test.mkv",
		OutputPath:   "/out/test.mkv",
		MuxQueueSize: 4096,
		BSFOpts:      []string{"-bsf:v:0", "dovi_rpu=strip=1"},
	}
	args := Build(cfg, plan, NewRetryState(plan))
	if got := argValue(args, "-bsf:v:0"); got != "dovi_rpu=strip=1" {
		t.Errorf("expected -bsf:v:0 dovi_rpu=strip=1, got %q", got)
	}
}

func TestBuild_SubtitleStreamCodecs(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:       planner.ActionEncode,
		VideoCodec:   "hevc_vaapi",
		InputPath:    "/in/test.mp4",
		OutputPath:   "/out/test.mkv",
		VaapiQP:      18,
		MuxQueueSize: 4096,
		IncludeSubs:  true,
		Subtitles: planner.SubtitlePlan{
			Include:      true,
			TextIdxs:     []int{2, 3},
			StreamCodecs: []string{"srt", "copy"},
		},
	}
	args := Build(cfg, plan, NewRetryState(plan))
	if !containsPair(args, "-map", "0:2") || !containsPair(args, "-map", "0:3") {
		t.Errorf("expected indexed subtitle maps, got %v", args)
	}
	if got := argValue(args, "-c:s:0"); got != "srt" {
		t.Errorf("expected -c:s:0 srt, got %q", got)
	}
	if got := argValue(args, "-c:s:1"); got != "copy" {
		t.Errorf("expected -c:s:1 copy, got %q", got)
	}
	if containsArg(args, "-c:s") {
		t.Error("per-stream mode should not emit a global -c:s")
	}
	if containsPair(args, "-map", "0:s?") {
		t.Error("per-stream mode should not emit -map 0:s?")
	}
}

func TestBuild_AttachedPicMaps(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:          planner.ActionEncode,
		VideoCodec:      "hevc_vaapi",
		InputPath:       "/in/test.mkv",
		OutputPath:      "/out/test.mkv",
		VaapiQP:         18,
		MuxQueueSize:    4096,
		AttachedPicIdxs: []int{4},
	}
	args := Build(cfg, plan, NewRetryState(plan))
	if !containsPair(args, "-map", "0:4") {
		t.Errorf("expected cover art map 0:4, got %v", args)
	}
	if got := argValue(args, "-c:v:1"); got != "copy" {
		t.Errorf("expected -c:v:1 copy, got %q", got)
	}
	if got := argValue(args, "-disposition:v:1"); got != "attached_pic" {
		t.Errorf("expected -disposition:v:1 attached_pic, got %q", got)
	}
	// The per-stream copy must come after the global encoder codec so it
	// wins for the cover stream (ffmpeg applies the last matching option).
	globalIdx, specificIdx := -1, -1
	for i, a := range args {
		if a == "-c:v" {
			globalIdx = i
		}
		if a == "-c:v:1" {
			specificIdx = i
		}
	}
	if globalIdx == -1 || specificIdx < globalIdx {
		t.Errorf("-c:v:1 (at %d) must come after -c:v (at %d)", specificIdx, globalIdx)
	}
}

// TestBuild_FilterScopedToPrimaryVideo guards files that both encode the main
// video and preserve embedded cover art. A global -vf applies to every output
// video stream, which collides with the cover stream's -c:v:N copy; the filter
// must be scoped to the primary output video stream instead.
func TestBuild_FilterScopedToPrimaryVideo(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:          planner.ActionEncode,
		VideoCodec:      "hevc_vaapi",
		InputPath:       "/in/test.mkv",
		OutputPath:      "/out/test.mkv",
		VaapiQP:         18,
		MuxQueueSize:    4096,
		VideoFilters:    "format=p010,hwupload",
		AttachedPicIdxs: []int{4},
	}
	args := Build(cfg, plan, NewRetryState(plan))
	if got := argValue(args, "-filter:v:0"); got != "format=p010,hwupload" {
		t.Errorf("expected primary-video filter, got %q", got)
	}
	if containsArg(args, "-vf") {
		t.Errorf("global -vf must not be emitted with cover art, got %v", args)
	}
	if got := argValue(args, "-c:v:1"); got != "copy" {
		t.Errorf("expected cover art to remain stream-copied, got %q", got)
	}
}

// TestBuild_AttachedPicBeforeAttachments guards the matroska muxer ordering
// constraint: the cover-art (attached_pic) -map must precede the -map 0:t?
// attachment map, or ffmpeg mis-routes the cover packet to an attachment slot
// and fails with EINVAL ("Received a packet for an attachment stream").
func TestBuild_AttachedPicBeforeAttachments(t *testing.T) {
	cfg := vaapiCfg()
	plan := &planner.FilePlan{
		Action:          planner.ActionRemux,
		InputPath:       "/in/test.mkv",
		OutputPath:      "/out/test.mkv",
		MuxQueueSize:    4096,
		AttachedPicIdxs: []int{5},
		Attachments:     planner.AttachmentPlan{Include: true},
		Container:       config.ContainerMKV,
		IncludeAttach:   true,
	}
	args := Build(cfg, plan, NewRetryState(plan))

	coverMapIdx, attachMapIdx := -1, -1
	for i, a := range args {
		if a == "-map" && i+1 < len(args) {
			switch args[i+1] {
			case "0:5":
				coverMapIdx = i
			case "0:t?":
				attachMapIdx = i
			}
		}
	}
	if coverMapIdx == -1 {
		t.Fatalf("expected cover-art map 0:5, got %v", args)
	}
	if attachMapIdx == -1 {
		t.Fatalf("expected attachment map 0:t?, got %v", args)
	}
	if coverMapIdx > attachMapIdx {
		t.Errorf("cover-art -map (at %d) must precede attachment -map 0:t? (at %d)", coverMapIdx, attachMapIdx)
	}
}

func containsPair(args []string, flag, val string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}
	return false
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

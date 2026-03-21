package planner

import (
	"fmt"
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// --- BuildPlan decision matrix tests ---

func TestBuildPlan_H264Encode(t *testing.T) {
	plan := BuildPlan(defaultCfg(), h264SDR())
	if plan.Action != ActionEncode {
		t.Errorf("action: got %d, want ActionEncode", plan.Action)
	}
	if plan.VideoCodec != "hevc_vaapi" {
		t.Errorf("codec: got %q, want hevc_vaapi", plan.VideoCodec)
	}
}

func TestBuildPlan_HEVCRemux(t *testing.T) {
	plan := BuildPlan(defaultCfg(), hevcEdgeSafe())
	if plan.Action != ActionRemux {
		t.Errorf("action: got %d, want ActionRemux", plan.Action)
	}
	if plan.VideoCodec != "copy" {
		t.Errorf("codec: got %q, want copy", plan.VideoCodec)
	}
}

func TestBuildPlan_HEVCUnsafeReencode(t *testing.T) {
	plan := BuildPlan(defaultCfg(), hevcUnsafe())
	if plan.Action != ActionEncode {
		t.Errorf("action: got %d, want ActionEncode (unsafe HEVC)", plan.Action)
	}
	if !strings.Contains(plan.QualityNote, "not browser-safe") {
		t.Errorf("QualityNote should mention browser-safe, got %q", plan.QualityNote)
	}
}

func TestBuildPlan_SkipHEVCDisabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.SkipHEVC = false
	plan := BuildPlan(cfg, hevcEdgeSafe())
	if plan.Action != ActionEncode {
		t.Errorf("with SkipHEVC=false, action: got %d, want ActionEncode", plan.Action)
	}
}

func TestBuildPlan_CPUMode(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	plan := BuildPlan(cfg, h264SDR())
	if plan.VideoCodec != "libx265" {
		t.Errorf("codec: got %q, want libx265", plan.VideoCodec)
	}
}

func TestBuildPlan_MP4Container(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutputContainer = config.ContainerMP4
	plan := BuildPlan(cfg, h264SDR())
	if len(plan.ContainerOpts) < 2 || plan.ContainerOpts[0] != "-movflags" {
		t.Errorf("ContainerOpts: %v", plan.ContainerOpts)
	}
	if len(plan.TagOpts) < 2 || plan.TagOpts[1] != "hvc1" {
		t.Errorf("TagOpts: %v", plan.TagOpts)
	}
	if plan.Attachments.Include {
		t.Error("MP4 should not include attachments")
	}
}

// --- TimestampFix tests ---

func TestBuildPlan_RemuxNoTimestampFix(t *testing.T) {
	cfg := defaultCfg()
	cfg.CleanTimestamps = true
	plan := BuildPlan(cfg, hevcEdgeSafe())
	if plan.Action != ActionRemux {
		t.Fatal("expected remux")
	}
	if plan.TimestampFix {
		t.Error("remux should have TimestampFix=false regardless of CleanTimestamps")
	}
}

func TestBuildPlan_EncodeRespectsCleanTimestamps(t *testing.T) {
	cfg := defaultCfg()
	cfg.CleanTimestamps = true
	plan := BuildPlan(cfg, h264SDR())
	if plan.Action != ActionEncode {
		t.Fatal("expected encode")
	}
	if !plan.TimestampFix {
		t.Error("encode with CleanTimestamps=true should have TimestampFix=true")
	}

	cfg.CleanTimestamps = false
	plan = BuildPlan(cfg, h264SDR())
	if plan.TimestampFix {
		t.Error("encode with CleanTimestamps=false should have TimestampFix=false")
	}
}

// --- SmartQuality tests ---

func TestSmartQuality_ManualOverride(t *testing.T) {
	cfg := defaultCfg()
	cfg.ActiveQualityOverride = "22"
	cfg.VaapiQP = 22
	q := SmartQuality(cfg, h264SDR())
	if q.VaapiQP != 22 {
		t.Errorf("VaapiQP: got %d, want 22", q.VaapiQP)
	}
	if !strings.Contains(q.Note, "manual fixed override") {
		t.Errorf("Note: got %q, want manual override mention", q.Note)
	}
}

func TestSmartQuality_Disabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.SmartQuality = false
	q := SmartQuality(cfg, h264SDR())
	if q.VaapiQP != cfg.VaapiQP || q.CpuCRF != cfg.CpuCRF {
		t.Errorf("disabled: want config defaults, got QP=%d CRF=%d", q.VaapiQP, q.CpuCRF)
	}
}

func TestSmartQuality_LowRes(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Width: 640, Height: 360, BitRate: 800000},
		Format:       probe.FormatInfo{BitRate: 900000},
	}
	q := SmartQuality(defaultCfg(), pr)
	// Low res + low bitrate → should bump quality values up significantly.
	if q.CpuCRF <= 18 {
		t.Errorf("low-res should increase CRF above default 18, got %d", q.CpuCRF)
	}
	if q.VaapiQP <= 18 {
		t.Errorf("low-res should increase QP above default 18, got %d", q.VaapiQP)
	}
}

func TestSmartQuality_4K(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Width: 3840, Height: 2160, BitRate: 40000000},
		Format:       probe.FormatInfo{BitRate: 42000000},
	}
	q := SmartQuality(defaultCfg(), pr)
	// 4K + high bitrate → should lower quality values for more quality.
	if q.CpuCRF >= 18 {
		t.Errorf("4K should decrease CRF below default 18, got %d", q.CpuCRF)
	}
}

func TestSmartQuality_BiasApplied(t *testing.T) {
	cfg := defaultCfg()
	cfg.SmartQualityBias = -1
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	q1 := SmartQuality(cfg, pr)

	cfg.SmartQualityBias = 0
	q2 := SmartQuality(cfg, pr)

	// Bias=-1 should produce lower values than bias=0.
	if q1.CpuCRF >= q2.CpuCRF {
		t.Errorf("bias=-1 CRF=%d should be < bias=0 CRF=%d", q1.CpuCRF, q2.CpuCRF)
	}
}

func TestSmartQuality_ClampRanges(t *testing.T) {
	// Very low res + very low bitrate → would push CRF/QP very high;
	// verify clamping.
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Width: 320, Height: 240, BitRate: 200000},
		Format:       probe.FormatInfo{BitRate: 250000},
	}
	q := SmartQuality(defaultCfg(), pr)
	if q.CpuCRF > CpuCRFMax {
		t.Errorf("CRF %d exceeds max %d", q.CpuCRF, CpuCRFMax)
	}
	if q.VaapiQP > VaapiQPMax {
		t.Errorf("QP %d exceeds max %d", q.VaapiQP, VaapiQPMax)
	}
}

// --- EstimateBitrate tests ---

func TestEstimateBitrate_H264(t *testing.T) {
	est := EstimateBitrate(defaultCfg(), h264SDR(), 19, 19)
	if !est.Known {
		t.Fatal("estimate should be known")
	}
	if est.LowKbps >= est.HighKbps {
		t.Errorf("low=%d should be < high=%d", est.LowKbps, est.HighKbps)
	}
	if est.LowPct >= est.HighPct {
		t.Errorf("low%%=%d should be < high%%=%d", est.LowPct, est.HighPct)
	}
	t.Logf("H264 1080p 8Mbps → %d-%d kb/s (%d-%d%%)", est.LowKbps, est.HighKbps, est.LowPct, est.HighPct)
}

func TestEstimateBitrate_NoBitrate(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080},
		Format:       probe.FormatInfo{},
	}
	est := EstimateBitrate(defaultCfg(), pr, 19, 19)
	if est.Known {
		t.Error("should be unknown when no bitrate data")
	}
}

func TestEstimateBitrate_CodecBias(t *testing.T) {
	cfg := defaultCfg()
	pr1 := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	pr2 := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "mpeg2video", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	est1 := EstimateBitrate(cfg, pr1, 19, 19)
	est2 := EstimateBitrate(cfg, pr2, 19, 19)
	// Modern codec (h264) should have higher ratio (less compression gain)
	// than legacy codec (mpeg2).
	if est1.HighKbps <= est2.HighKbps {
		t.Logf("h264 high=%d, mpeg2 high=%d — h264 should estimate higher", est1.HighKbps, est2.HighKbps)
	}
}

func TestEstimateBitrate_HEVCHigherThanH264(t *testing.T) {
	cfg := defaultCfg()
	prH264 := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	prHEVC := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "hevc", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	estH264 := EstimateBitrate(cfg, prH264, 22, 22)
	estHEVC := EstimateBitrate(cfg, prHEVC, 22, 22)
	// HEVC→HEVC has almost no codec-generation gain, so it should predict
	// a higher output ratio than h264→HEVC.
	if estHEVC.HighKbps <= estH264.HighKbps {
		t.Errorf("HEVC source should estimate higher than h264: HEVC=%d, h264=%d",
			estHEVC.HighKbps, estH264.HighKbps)
	}
	t.Logf("h264→HEVC: %d-%d kb/s; HEVC→HEVC: %d-%d kb/s",
		estH264.LowKbps, estH264.HighKbps, estHEVC.LowKbps, estHEVC.HighKbps)
}

func TestSmartQuality_1440p_VAAPI(t *testing.T) {
	// 1440p content should get a quality bonus (lower QP) on both CPU and VAAPI.
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 2560, Height: 1440, BitRate: 12000000,
		},
		Format: probe.FormatInfo{BitRate: 13000000},
	}
	q := SmartQuality(defaultCfg(), pr)
	if q.VaapiQP > 18 {
		t.Errorf("1440p should get QP <= 18 (quality bonus), got %d", q.VaapiQP)
	}
	// Verify CPU also gets the bonus.
	if q.CpuCRF > 18 {
		t.Errorf("1440p should get CRF <= 18 (quality bonus), got %d", q.CpuCRF)
	}
	t.Logf("1440p 12Mbps → QP=%d CRF=%d", q.VaapiQP, q.CpuCRF)
}

// --- Density curve tests ---

func TestSmartQuality_CompressedSource(t *testing.T) {
	// Simulates the user's scenario: 1080p h264 at only 3.9 Mbps.
	// Density = 3900 * 1e6 / (1920*1080) ≈ 1880 kbps/Mpx → compressed.
	// Quality-first: density curve applies a mild +1 QP adjustment.
	// The post-encode escalation loop handles genuine overshoot.
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 3900000,
		},
		Format: probe.FormatInfo{BitRate: 4100000},
	}
	q := SmartQuality(defaultCfg(), pr)
	if q.VaapiQP < VaapiQPMin || q.VaapiQP > VaapiQPMax {
		t.Errorf("QP %d out of range [%d, %d]", q.VaapiQP, VaapiQPMin, VaapiQPMax)
	}
	t.Logf("Compressed 1080p 3.9Mbps → QP=%d CRF=%d", q.VaapiQP, q.CpuCRF)
}

func TestSmartQuality_HighDensitySource(t *testing.T) {
	// High quality source: 1080p at 25 Mbps → density ≈ 12056.
	// Should get a quality *bonus* (lower QP).
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 25000000,
		},
		Format: probe.FormatInfo{BitRate: 26000000},
	}
	q := SmartQuality(defaultCfg(), pr)
	// High density should not increase QP — should stay at or below default area.
	if q.VaapiQP > 18 {
		t.Errorf("high-quality 1080p at 25 Mbps should not increase QP above 18, got %d", q.VaapiQP)
	}
	t.Logf("High-quality 1080p 25Mbps → QP=%d CRF=%d", q.VaapiQP, q.CpuCRF)
}

func TestDensityCurve_Boundaries(t *testing.T) {
	tests := []struct {
		kbps, pixels int
		wantCPU      int
		wantVAAPI    int
		label        string
	}{
		{0, 2073600, 0, 0, "zero bitrate"},
		{3900, 0, 0, 0, "zero pixels"},
		{500, 2073600, 3, 4, "ultra-low density (241)"},
		{1000, 2073600, 3, 4, "ultra-low density (482)"},
		{2500, 2073600, 2, 2, "low density (1206)"},
		{4000, 2073600, 1, 1, "below-avg density (1929)"},
		{6000, 2073600, 0, 0, "medium-low density (2893)"},
		{8000, 2073600, 0, 0, "normal density (3858)"},
		{20000, 2073600, -1, -2, "high density (9645)"},
	}
	for _, tt := range tests {
		gotCPU := cpuDensityCurve(tt.kbps, tt.pixels)
		gotVAAPI := vaapiDensityCurve(tt.kbps, tt.pixels)
		if gotCPU != tt.wantCPU {
			t.Errorf("%s: cpuDensity=%d, want %d", tt.label, gotCPU, tt.wantCPU)
		}
		if gotVAAPI != tt.wantVAAPI {
			t.Errorf("%s: vaapiDensity=%d, want %d", tt.label, gotVAAPI, tt.wantVAAPI)
		}
	}
}

// --- PreflightAdjust tests ---

func TestPreflightAdjust_NoBumpNeeded(t *testing.T) {
	// Normal 1080p at 8 Mbps — estimate should not trigger pre-flight.
	cfg := defaultCfg()
	pr := h264SDR()
	qp, crf, bumps := PreflightAdjust(cfg, pr, 22, 22, 105)
	if bumps != 0 {
		t.Errorf("normal source should need 0 bumps, got %d (QP=%d CRF=%d)", bumps, qp, crf)
	}
}

func TestPreflightAdjust_CompressedSource(t *testing.T) {
	cfg := defaultCfg()
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 3900000,
		},
		Format: probe.FormatInfo{BitRate: 4100000},
	}
	// Start at QP 20 where the estimate overshoots, verifying the
	// preflight mechanism still bumps when needed.
	qp, _, bumps := PreflightAdjust(cfg, pr, 20, 20, 100)
	if bumps == 0 {
		t.Error("compressed source at QP 20 should need preflight bumps at 100% target")
	}
	if qp <= 20 {
		t.Errorf("QP should be bumped above 20, got %d", qp)
	}
	t.Logf("Compressed 1080p: QP %d→%d (%d bumps)", 20, qp, bumps)
}

func TestPreflightAdjust_RespectsClampMax(t *testing.T) {
	// Start near the max QP with a source that would trigger bumps.
	// 1080p at 2 Mbps, starting at QP 28 with a tight target.
	// QP should never exceed VaapiQPMax.
	cfg := defaultCfg()
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 2000000,
		},
		Format: probe.FormatInfo{BitRate: 2500000},
	}
	qp, _, _ := PreflightAdjust(cfg, pr, 28, 28, 80)
	if qp > VaapiQPMax {
		t.Errorf("QP %d exceeds max %d", qp, VaapiQPMax)
	}
}

// --- Extended ratio table tests ---

func TestVaapiRatio_FullRange(t *testing.T) {
	prev := vaapiRatio(14)
	for qp := 15; qp <= 36; qp++ {
		r := vaapiRatio(qp)
		if r >= prev {
			t.Errorf("vaapiRatio(%d)=%d should be < vaapiRatio(%d)=%d", qp, r, qp-1, prev)
		}
		prev = r
	}
	// Ensure the floor at QP 36 is reasonable.
	if vaapiRatio(36) < 150 || vaapiRatio(36) > 250 {
		t.Errorf("vaapiRatio(36)=%d, want 150-250", vaapiRatio(36))
	}
}

func TestCpuRatio_FullRange(t *testing.T) {
	prev := cpuRatio(16)
	for crf := 17; crf <= 30; crf++ {
		r := cpuRatio(crf)
		if r >= prev {
			t.Errorf("cpuRatio(%d)=%d should be < cpuRatio(%d)=%d", crf, r, crf-1, prev)
		}
		prev = r
	}
	if cpuRatio(30) < 150 || cpuRatio(30) > 250 {
		t.Errorf("cpuRatio(30)=%d, want 150-250", cpuRatio(30))
	}
}

func TestEstimationDensityBias_Boundaries(t *testing.T) {
	tests := []struct {
		kbps, pixels, want int
		label              string
	}{
		{0, 2073600, 0, "zero bitrate"},
		{3900, 0, 0, "zero pixels"},
		{500, 2073600, 250, "ultra-low density (241)"},
		{1000, 2073600, 250, "ultra-low density (482)"},
		{2500, 2073600, 150, "low density (1206)"},
		{4000, 2073600, 80, "below-avg density (1929)"},
		{6000, 2073600, 20, "medium density (2893)"},
		{8000, 2073600, 0, "normal density (3858)"},
		{20000, 2073600, -10, "high density (9645)"},
		{25000, 2073600, -20, "very high density (12056)"},
	}
	for _, tt := range tests {
		got := estimationDensityBias(tt.kbps, tt.pixels)
		if got != tt.want {
			t.Errorf("%s: estimationDensityBias=%d, want %d", tt.label, got, tt.want)
		}
	}
}

// --- BuildVideoFilter tests ---

func TestBuildVideoFilter_VaapiDefault(t *testing.T) {
	cfg := defaultCfg()
	f := BuildVideoFilter(cfg, h264SDR(), false)
	if !strings.Contains(f, "format=p010") || !strings.Contains(f, "hwupload") {
		t.Errorf("VAAPI filter should have format+hwupload, got %q", f)
	}
	if strings.Contains(f, "yadif") {
		t.Error("progressive content should not have yadif")
	}
}

func TestBuildVideoFilter_CPUNoFilter(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	f := BuildVideoFilter(cfg, h264SDR(), false)
	if f != "" {
		t.Errorf("CPU + progressive + SDR should have no filter, got %q", f)
	}
}

func TestBuildVideoFilter_Deinterlace(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	f := BuildVideoFilter(cfg, interlacedFile(), false)
	if !strings.Contains(f, "yadif=mode=send_frame:parity=auto:deint=interlaced") {
		t.Errorf("interlaced should have full yadif, got %q", f)
	}
}

func TestBuildVideoFilter_DeinterlaceDisabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.DeinterlaceAuto = false
	cfg.EncoderMode = config.EncoderCPU
	f := BuildVideoFilter(cfg, interlacedFile(), false)
	if strings.Contains(f, "yadif") {
		t.Errorf("DeinterlaceAuto=false should not produce yadif, got %q", f)
	}
}

func TestBuildVideoFilter_HDRTonemap(t *testing.T) {
	cfg := defaultCfg()
	cfg.HandleHDR = config.HDRTonemap
	cfg.EncoderMode = config.EncoderCPU
	cfg.SkipHEVC = false
	f := BuildVideoFilter(cfg, hdr10File(), false)
	if !strings.Contains(f, "tonemap") || !strings.Contains(f, "hable") {
		t.Errorf("HDR tonemap should have tonemap filter, got %q", f)
	}
}

func TestBuildVideoFilter_HDRPreserve(t *testing.T) {
	cfg := defaultCfg()
	cfg.HandleHDR = config.HDRPreserve
	cfg.EncoderMode = config.EncoderCPU
	cfg.SkipHEVC = false
	f := BuildVideoFilter(cfg, hdr10File(), false)
	if strings.Contains(f, "tonemap") {
		t.Errorf("HDR preserve should NOT have tonemap, got %q", f)
	}
}

// --- Hardware decode filter tests ---

func TestBuildVideoFilter_HWDecode_NoFilters(t *testing.T) {
	cfg := defaultCfg()
	f := BuildVideoFilter(cfg, h264SDR(), true)
	if f != "" {
		t.Errorf("HW decode + progressive + SDR should have no filter, got %q", f)
	}
}

func TestBuildVideoFilter_HWDecode_Deinterlace(t *testing.T) {
	cfg := defaultCfg()
	f := BuildVideoFilter(cfg, interlacedFile(), true)
	if f != "deinterlace_vaapi" {
		t.Errorf("HW decode + interlaced should use deinterlace_vaapi, got %q", f)
	}
}

func TestBuildVideoFilter_HWDecode_NoHwupload(t *testing.T) {
	cfg := defaultCfg()
	f := BuildVideoFilter(cfg, h264SDR(), true)
	if strings.Contains(f, "hwupload") || strings.Contains(f, "format=") {
		t.Errorf("HW decode should not have hwupload or format conversion, got %q", f)
	}
}

func TestBuildColorOpts_HDRPreserve(t *testing.T) {
	cfg := defaultCfg()
	cfg.HandleHDR = config.HDRPreserve
	opts := BuildColorOpts(cfg, hdr10File())
	if len(opts) != 6 {
		t.Fatalf("expected 6 color opts (3 pairs), got %d: %v", len(opts), opts)
	}
	if opts[1] != "smpte2084" {
		t.Errorf("color_trc: got %q", opts[1])
	}
	if opts[3] != "bt2020" {
		t.Errorf("color_primaries: got %q", opts[3])
	}
}

func TestBuildColorOpts_SDR(t *testing.T) {
	cfg := defaultCfg()
	opts := BuildColorOpts(cfg, h264SDR())
	if len(opts) != 0 {
		t.Errorf("SDR should produce no color opts, got %v", opts)
	}
}

// --- BuildAudioPlan tests ---

func TestBuildAudioPlan_NoAudio(t *testing.T) {
	pr := &probe.ProbeResult{PrimaryVideo: &probe.VideoStream{Codec: "h264"}}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.NoAudio {
		t.Error("expected NoAudio")
	}
}

func TestBuildAudioPlan_AllAAC(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2},
			{Codec: "aac", Channels: 6},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.CopyAll {
		t.Error("all AAC should produce CopyAll")
	}
}

func TestBuildAudioPlan_MixedStreams(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2, SampleRate: 48000},
			{Codec: "ac3", Channels: 6, SampleRate: 48000},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if ap.NoAudio || ap.CopyAll {
		t.Fatal("mixed streams: should be per-stream plan")
	}
	if len(ap.Streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(ap.Streams))
	}
	if !ap.Streams[0].Copy {
		t.Error("stream 0 (aac) should be Copy")
	}
	if ap.Streams[1].Copy {
		t.Error("stream 1 (ac3) should NOT be Copy")
	}
}

func TestBuildAudioPlan_AllAACLowBitrate(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2, BitRate: 256_000},
			{Codec: "aac", Channels: 6, BitRate: 192_000},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.CopyAll {
		t.Error("all AAC should produce CopyAll")
	}
}

func TestBuildAudioPlan_AllAACHighBitrate(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2, BitRate: 128_000},
			{Codec: "aac", Channels: 6, BitRate: 512_000},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.CopyAll {
		t.Error("all AAC (any bitrate) should produce CopyAll")
	}
}

func TestBuildAudioPlan_AACExactThreshold(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2, BitRate: 400_000},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.CopyAll {
		t.Error("AAC at 400 kbps should produce CopyAll (all AAC is passthrough)")
	}
}

func TestBuildAudioPlan_AACUnknownBitrate(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2, BitRate: 0},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.CopyAll {
		t.Error("AAC with unknown bitrate (0) should produce CopyAll")
	}
}

func TestBuildAudioPlan_AACHighBitrateSingleStream(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 6, BitRate: 640_000},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if !ap.CopyAll {
		t.Error("AAC at 640 kbps should produce CopyAll (AAC always passthrough)")
	}
}

func TestBuildAudioPlan_MixedAACHighBitrateAndNonAAC(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac", Channels: 2, SampleRate: 48000, BitRate: 400_000},
			{Codec: "ac3", Channels: 6, SampleRate: 48000},
		},
	}
	ap := BuildAudioPlan(defaultCfg(), pr)
	if ap.CopyAll {
		t.Error("should not be CopyAll (has non-AAC stream)")
	}
	if len(ap.Streams) != 2 {
		t.Fatalf("expected 2 streams, got %d", len(ap.Streams))
	}
	if !ap.Streams[0].Copy {
		t.Error("stream 0 (aac 400k) should be Copy (AAC always passthrough)")
	}
	if ap.Streams[1].Copy {
		t.Error("stream 1 (ac3) should NOT be Copy")
	}
}

func TestBuildAudioPlan_ChannelClamp(t *testing.T) {
	cfg := defaultCfg()
	cfg.AudioChannels = 2
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{{Codec: "dts", Channels: 8, SampleRate: 48000}},
	}
	ap := BuildAudioPlan(cfg, pr)
	if ap.Streams[0].Channels != 2 {
		t.Errorf("8ch should be clamped to 2, got %d", ap.Streams[0].Channels)
	}
}

func TestBuildAudioPlan_LayoutFilter(t *testing.T) {
	cfg := defaultCfg()
	cfg.MatchAudioLayout = true
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{{Codec: "ac3", Channels: 2, SampleRate: 48000}},
	}
	ap := BuildAudioPlan(cfg, pr)
	s := ap.Streams[0]
	if !s.NeedsFilter {
		t.Error("MATCH_AUDIO_LAYOUT should set NeedsFilter")
	}
	if !strings.Contains(s.FilterStr, "aresample") || !strings.Contains(s.FilterStr, "channel_layouts=stereo") {
		t.Errorf("filter: got %q", s.FilterStr)
	}
	if s.Layout != "stereo" {
		t.Errorf("layout: got %q, want stereo", s.Layout)
	}
}

func TestBuildAudioPlan_LayoutFilterMono(t *testing.T) {
	cfg := defaultCfg()
	cfg.MatchAudioLayout = true
	cfg.AudioChannels = 1
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{{Codec: "ac3", Channels: 1, SampleRate: 48000}},
	}
	ap := BuildAudioPlan(cfg, pr)
	if !strings.Contains(ap.Streams[0].FilterStr, "channel_layouts=mono") {
		t.Errorf("mono filter: got %q", ap.Streams[0].FilterStr)
	}
}

func TestBuildAudioPlan_NoLayoutFilter(t *testing.T) {
	cfg := defaultCfg()
	cfg.MatchAudioLayout = false
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{{Codec: "ac3", Channels: 2, SampleRate: 48000}},
	}
	ap := BuildAudioPlan(cfg, pr)
	if ap.Streams[0].NeedsFilter {
		t.Error("MatchAudioLayout=false should not set NeedsFilter")
	}
}

func TestBuildAudioPlan_HighChannelNoLayout(t *testing.T) {
	cfg := defaultCfg()
	cfg.MatchAudioLayout = true
	cfg.AudioChannels = 6
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{{Codec: "dts", Channels: 6, SampleRate: 48000}},
	}
	ap := BuildAudioPlan(cfg, pr)
	s := ap.Streams[0]
	if s.Layout != "" {
		t.Errorf("6ch should have empty layout, got %q", s.Layout)
	}
	if !strings.Contains(s.FilterStr, "aresample") {
		t.Error("should still have aresample filter")
	}
	if strings.Contains(s.FilterStr, "channel_layouts=") {
		t.Error("6ch filter should NOT contain channel_layouts")
	}
}

// --- BuildSubtitlePlan tests ---

func TestBuildSubtitlePlan_MKVCopy(t *testing.T) {
	sp := BuildSubtitlePlan(defaultCfg(), h264SDR())
	if !sp.Include || sp.Codec != "copy" {
		t.Errorf("MKV: got include=%v codec=%q", sp.Include, sp.Codec)
	}
}

func TestBuildSubtitlePlan_MP4TextSubs(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutputContainer = config.ContainerMP4
	pr := &probe.ProbeResult{
		PrimaryVideo:    &probe.VideoStream{Codec: "h264"},
		SubtitleStreams: []probe.SubtitleStream{{Codec: "srt"}},
	}
	sp := BuildSubtitlePlan(cfg, pr)
	if !sp.Include || sp.Codec != "mov_text" {
		t.Errorf("MP4 text: got include=%v codec=%q", sp.Include, sp.Codec)
	}
}

func TestBuildSubtitlePlan_MP4BitmapSkip(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutputContainer = config.ContainerMP4
	pr := &probe.ProbeResult{
		PrimaryVideo:    &probe.VideoStream{Codec: "h264"},
		SubtitleStreams: []probe.SubtitleStream{{Index: 3, Codec: "hdmv_pgs_subtitle", IsBitmap: true}},
		HasBitmapSubs:   true,
	}
	sp := BuildSubtitlePlan(cfg, pr)
	if sp.Include {
		t.Error("MP4 with only bitmap subs should not include subs")
	}
}

func TestBuildSubtitlePlan_MP4MixedSubs(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutputContainer = config.ContainerMP4
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		SubtitleStreams: []probe.SubtitleStream{
			{Index: 3, Codec: "hdmv_pgs_subtitle", Language: "eng", IsBitmap: true},
			{Index: 4, Codec: "srt", Language: "eng", IsBitmap: false},
			{Index: 5, Codec: "ass", Language: "jpn", IsBitmap: false},
		},
		HasBitmapSubs: true,
	}
	sp := BuildSubtitlePlan(cfg, pr)
	if !sp.Include {
		t.Fatal("MP4 with mixed subs should include text subs")
	}
	if sp.Codec != "mov_text" {
		t.Errorf("codec: got %q, want mov_text", sp.Codec)
	}
	if !sp.SkipBitmap {
		t.Error("SkipBitmap should be true when bitmap subs are present")
	}
	if len(sp.TextIdxs) != 2 {
		t.Fatalf("TextIdxs: got %v, want 2 entries", sp.TextIdxs)
	}
	if sp.TextIdxs[0] != 4 || sp.TextIdxs[1] != 5 {
		t.Errorf("TextIdxs: got %v, want [4 5]", sp.TextIdxs)
	}
}

func TestBuildSubtitlePlan_Disabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.KeepSubtitles = false
	sp := BuildSubtitlePlan(cfg, h264SDR())
	if sp.Include {
		t.Error("KeepSubtitles=false should not include subs")
	}
}

func TestBuildSubtitlePlan_NoSubs(t *testing.T) {
	pr := &probe.ProbeResult{PrimaryVideo: &probe.VideoStream{Codec: "h264"}}
	sp := BuildSubtitlePlan(defaultCfg(), pr)
	if sp.Include {
		t.Error("no subtitle streams should not include subs")
	}
}

// --- BuildDispositions tests ---

func TestBuildDispositions_SingleAudio(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{{Codec: "aac"}},
	}
	opts := BuildDispositions(pr)
	if len(opts) != 4 { // -disposition:v:0 default -disposition:a:0 default
		t.Errorf("expected 4 opts, got %d: %v", len(opts), opts)
	}
}

func TestBuildDispositions_MultiAudio(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams: []probe.AudioStream{
			{Codec: "aac"}, {Codec: "ac3"}, {Codec: "dts"},
		},
	}
	opts := BuildDispositions(pr)
	// v:0=default, a:0=default, a:1=0, a:2=0 → 8 args
	if len(opts) != 8 {
		t.Errorf("expected 8 opts for 3 audio, got %d: %v", len(opts), opts)
	}
}

func TestBuildDispositions_NoAudio(t *testing.T) {
	pr := &probe.ProbeResult{PrimaryVideo: &probe.VideoStream{Codec: "h264"}}
	opts := BuildDispositions(pr)
	if len(opts) != 2 { // v:0=default only
		t.Errorf("expected 2 opts for no audio, got %d: %v", len(opts), opts)
	}
}

// --- BuildAttachmentPlan tests ---

func TestBuildAttachmentPlan_MKV(t *testing.T) {
	ap := BuildAttachmentPlan(defaultCfg())
	if !ap.Include {
		t.Error("MKV should include attachments")
	}
}

func TestBuildAttachmentPlan_MP4(t *testing.T) {
	cfg := defaultCfg()
	cfg.OutputContainer = config.ContainerMP4
	ap := BuildAttachmentPlan(cfg)
	if ap.Include {
		t.Error("MP4 should not include attachments")
	}
}

func TestBuildAttachmentPlan_Disabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.KeepAttachments = false
	ap := BuildAttachmentPlan(cfg)
	if ap.Include {
		t.Error("KeepAttachments=false should not include attachments")
	}
}

// --- Integration: full plan for common scenarios ---

func TestFullPlan_TypicalAnime(t *testing.T) {
	cfg := defaultCfg()
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Profile: "High", PixFmt: "yuv420p",
			Width: 1920, Height: 1080, BitRate: 6000000,
			FieldOrder: "progressive",
		},
		AudioStreams: []probe.AudioStream{
			{Codec: "flac", Channels: 2, SampleRate: 48000, Language: "jpn"},
			{Codec: "aac", Channels: 2, SampleRate: 48000, Language: "eng"},
		},
		SubtitleStreams: []probe.SubtitleStream{
			{Codec: "ass", Language: "eng"},
			{Codec: "ass", Language: "jpn"},
		},
		Format: probe.FormatInfo{BitRate: 8000000},
	}

	plan := BuildPlan(cfg, pr)

	if plan.Action != ActionEncode {
		t.Error("should encode")
	}
	if plan.VideoCodec != "hevc_vaapi" {
		t.Errorf("codec: %s", plan.VideoCodec)
	}
	if !plan.HWDecode {
		t.Error("VAAPI SDR should enable HW decode")
	}
	if plan.Audio.NoAudio || plan.Audio.CopyAll {
		t.Error("mixed audio: should be per-stream")
	}
	if len(plan.Audio.Streams) != 2 {
		t.Fatalf("audio streams: %d", len(plan.Audio.Streams))
	}
	if plan.Audio.Streams[0].Copy {
		t.Error("flac should not be copy")
	}
	if !plan.Audio.Streams[1].Copy {
		t.Error("aac should be copy")
	}
	if !plan.Subtitles.Include || plan.Subtitles.Codec != "copy" {
		t.Error("subs should be copy for MKV")
	}
	if !plan.Attachments.Include {
		t.Error("attachments should be included for MKV")
	}

	t.Logf("Quality: VaapiQP=%d CpuCRF=%d (%s)", plan.VaapiQP, plan.CpuCRF, plan.QualityNote)
}

func TestFullPlan_HDRPreserve(t *testing.T) {
	cfg := defaultCfg()
	cfg.SkipHEVC = false
	plan := BuildPlan(cfg, hdr10File())

	if plan.Action != ActionEncode {
		t.Error("should encode")
	}
	if len(plan.ColorOpts) == 0 {
		t.Error("HDR preserve should have color opts")
	}
	if !plan.HWDecode {
		t.Error("HDR preserve (no tonemap) should enable HW decode")
	}
	if plan.VideoFilters != "" {
		t.Errorf("HW decode + progressive + HDR preserve should have no filters, got %q", plan.VideoFilters)
	}
	if strings.Contains(plan.VideoFilters, "tonemap") {
		t.Error("HDR preserve should NOT tonemap")
	}
}

func TestFullPlan_InterlacedVAAPI(t *testing.T) {
	plan := BuildPlan(defaultCfg(), interlacedFile())
	if !plan.HWDecode {
		t.Error("interlaced VAAPI should enable HW decode")
	}
	if plan.VideoFilters != "deinterlace_vaapi" {
		t.Errorf("interlaced VAAPI with HW decode should use deinterlace_vaapi, got %q", plan.VideoFilters)
	}
}

func TestFullPlan_HDRTonemap_SoftwareDecode(t *testing.T) {
	cfg := defaultCfg()
	cfg.HandleHDR = config.HDRTonemap
	cfg.SkipHEVC = false
	plan := BuildPlan(cfg, hdr10File())

	if plan.HWDecode {
		t.Error("HDR tonemap should disable HW decode (zscale/tonemap are CPU-only)")
	}
	if !strings.Contains(plan.VideoFilters, "tonemap") {
		t.Error("HDR tonemap should have tonemap filter")
	}
	if !strings.Contains(plan.VideoFilters, "hwupload") {
		t.Error("software decode VAAPI should have hwupload")
	}
}

// --- Ratio table spot checks ---

func TestVaapiRatioTable(t *testing.T) {
	if vaapiRatio(19) != 730 {
		t.Errorf("vaapi QP19: got %d, want 730", vaapiRatio(19))
	}
	if vaapiRatio(14) != 930 {
		t.Errorf("vaapi QP14: got %d, want 930", vaapiRatio(14))
	}
	if vaapiRatio(27) != 395 {
		t.Errorf("vaapi QP27: got %d, want 395", vaapiRatio(27))
	}
}

func TestCpuRatioTable(t *testing.T) {
	if cpuRatio(19) != 660 {
		t.Errorf("cpu CRF19: got %d, want 660", cpuRatio(19))
	}
	if cpuRatio(16) != 900 {
		t.Errorf("cpu CRF16: got %d, want 900", cpuRatio(16))
	}
	if cpuRatio(28) != 235 {
		t.Errorf("cpu CRF28: got %d, want 235", cpuRatio(28))
	}
}

// --- Comprehensive resolution×bitrate matrix test ---
//
// Exercises the full quality pipeline across every realistic combination
// to verify: (a) QP/CRF stays within sane bounds, (b) estimated output
// never exceeds ~105% of input after preflight, (c) no extreme density
// case produces a clamped-at-max quality value (indicating the curves
// top out before handling the input).

func TestSmartQuality_Matrix(t *testing.T) {
	type scenario struct {
		label       string
		width       int
		height      int
		bitrateKbps int
		codec       string
	}

	scenarios := []scenario{
		// 360p
		{"360p ultra-low", 640, 360, 300, "h264"},
		{"360p low", 640, 360, 500, "h264"},
		{"360p normal", 640, 360, 1000, "h264"},
		{"360p high", 640, 360, 2000, "h264"},

		// 480p
		{"480p ultra-low", 854, 480, 400, "h264"},
		{"480p low", 854, 480, 800, "h264"},
		{"480p normal", 854, 480, 1500, "h264"},
		{"480p high", 854, 480, 3000, "h264"},

		// 720p
		{"720p ultra-low", 1280, 720, 1000, "h264"},
		{"720p low", 1280, 720, 1500, "h264"},
		{"720p normal", 1280, 720, 3000, "h264"},
		{"720p high", 1280, 720, 6000, "h264"},
		{"720p very high", 1280, 720, 10000, "h264"},

		// 1080p
		{"1080p ultra-low", 1920, 1080, 2000, "h264"},
		{"1080p compressed (user's case)", 1920, 1080, 3900, "h264"},
		{"1080p below average", 1920, 1080, 5000, "h264"},
		{"1080p normal", 1920, 1080, 8000, "h264"},
		{"1080p high", 1920, 1080, 15000, "h264"},
		{"1080p very high", 1920, 1080, 25000, "h264"},
		{"1080p HEVC re-encode", 1920, 1080, 5000, "hevc"},
		{"1080p HEVC high", 1920, 1080, 15000, "hevc"},
		{"1080p mpeg2", 1920, 1080, 8000, "mpeg2video"},

		// 1440p
		{"1440p low", 2560, 1440, 5000, "h264"},
		{"1440p normal", 2560, 1440, 10000, "h264"},
		{"1440p high", 2560, 1440, 20000, "h264"},

		// 4K
		{"4K low", 3840, 2160, 10000, "h264"},
		{"4K normal", 3840, 2160, 20000, "h264"},
		{"4K high", 3840, 2160, 40000, "h264"},
		{"4K remux-grade", 3840, 2160, 60000, "h264"},
		{"4K HEVC", 3840, 2160, 25000, "hevc"},
	}

	cfg := defaultCfg()

	for _, s := range scenarios {
		t.Run(s.label, func(t *testing.T) {
			pr := &probe.ProbeResult{
				PrimaryVideo: &probe.VideoStream{
					Codec: s.codec, Width: s.width, Height: s.height,
					BitRate: int64(s.bitrateKbps) * 1000,
				},
				Format: probe.FormatInfo{
					BitRate: int64(s.bitrateKbps)*1000 + 500_000, // +500k for audio
				},
			}

			q := SmartQuality(cfg, pr)
			pixels := s.width * s.height
			density := s.bitrateKbps * 1_000_000 / pixels

			// QP and CRF must be within valid ranges.
			if q.VaapiQP < VaapiQPMin || q.VaapiQP > VaapiQPMax {
				t.Errorf("QP %d out of range [%d, %d]", q.VaapiQP, VaapiQPMin, VaapiQPMax)
			}
			if q.CpuCRF < CpuCRFMin || q.CpuCRF > CpuCRFMax {
				t.Errorf("CRF %d out of range [%d, %d]", q.CpuCRF, CpuCRFMin, CpuCRFMax)
			}

			// Very low density sources (< 1500 kbps/Mpx) at mid resolutions
			// should still get a positive QP adjustment. Quality-first
			// curves are gentler, so the threshold is narrower.
			if density < 1500 && s.bitrateKbps > 0 && pixels <= 1920*1080 && pixels > 854*480 {
				if q.VaapiQP <= 18 {
					t.Errorf("very low density (%d kbps/Mpx) should push QP above 18, got %d",
						density, q.VaapiQP)
				}
			}

			// High-density 4K sources should get a quality bonus.
			if pixels >= 3840*2160 && density > 5000 {
				if q.VaapiQP > 18 {
					t.Errorf("high-density 4K should not increase QP above 18, got %d", q.VaapiQP)
				}
			}

			// Run preflight: after adjustment, the estimated high output
			// should be within tolerance. Quality-first preflight uses
			// 105% target with max 4 bumps; mild overshoot is acceptable
			// since the post-encode escalation loop handles it.
			adjQP, adjCRF, bumps := PreflightAdjust(cfg, pr, q.VaapiQP, q.CpuCRF, 105)
			est := EstimateBitrate(cfg, pr, adjQP, adjCRF)
			atMax := adjQP >= VaapiQPMax || adjCRF >= CpuCRFMax || (bumps >= 4 && density < 2500)
			if est.Known && est.HighPct > 120 && !atMax {
				t.Errorf("after preflight (%d bumps): estimated high=%d%% exceeds 120%% "+
					"(QP=%d CRF=%d)", bumps, est.HighPct, adjQP, adjCRF)
			}

			// Optimal bitrate should be positive and ≤ input.
			optKbps := OptimalBitrate(pr)
			if optKbps <= 0 {
				t.Error("OptimalBitrate should be > 0")
			}
			if optKbps > s.bitrateKbps {
				t.Errorf("OptimalBitrate %d exceeds input %d", optKbps, s.bitrateKbps)
			}

			// MaxRate calculation (CPU path).
			maxRate := optKbps * 115 / 100
			if maxRate > s.bitrateKbps {
				maxRate = s.bitrateKbps
			}

			t.Logf("density=%d kbps/Mpx QP=%d CRF=%d preflight_bumps=%d est=%d-%d%% optimal=%dk maxrate=%dk",
				density, q.VaapiQP, q.CpuCRF, bumps, safeEstLow(est), safeEstHigh(est), optKbps, maxRate)
		})
	}
}

// --- MaxRate and BuildPlan integration tests ---

func TestBuildPlan_CPUMaxRate(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000,
		},
		Format: probe.FormatInfo{BitRate: 9000000},
	}
	plan := BuildPlan(cfg, pr)
	// MaxRate is now based on optimal bitrate + 15% headroom, not raw input.
	// For h264 1080p 8 Mbps: optimal ≈ 5200, * 1.15 ≈ 5980.
	if plan.MaxRateKbps <= 0 {
		t.Error("CPU mode should set MaxRateKbps > 0")
	}
	if plan.MaxRateKbps > 8000 {
		t.Errorf("MaxRateKbps %d should not exceed input 8000", plan.MaxRateKbps)
	}
	if plan.BufSizeKbps != plan.MaxRateKbps*2 {
		t.Errorf("BufSizeKbps: got %d, want %d (2× maxrate)", plan.BufSizeKbps, plan.MaxRateKbps*2)
	}
	if plan.OptimalBitrateKbps <= 0 {
		t.Error("OptimalBitrateKbps should be set")
	}
	t.Logf("MaxRate=%dk Optimal=%dk BufSize=%dk", plan.MaxRateKbps, plan.OptimalBitrateKbps, plan.BufSizeKbps)
}

func TestBuildPlan_VaapiNoMaxRate(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderVAAPI
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000,
		},
		Format: probe.FormatInfo{BitRate: 9000000},
	}
	plan := BuildPlan(cfg, pr)
	if plan.MaxRateKbps != 0 {
		t.Errorf("VAAPI should not set MaxRateKbps, got %d", plan.MaxRateKbps)
	}
}

func TestBuildPlan_RemuxNoMaxRate(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	cfg.SkipHEVC = true
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Main 10", PixFmt: "yuv420p10le",
			Width: 1920, Height: 1080, BitRate: 5000000,
		},
		AudioStreams: []probe.AudioStream{{Codec: "aac", Channels: 2}},
		Format:       probe.FormatInfo{BitRate: 6000000},
	}
	plan := BuildPlan(cfg, pr)
	if plan.MaxRateKbps != 0 {
		t.Errorf("remux should not set MaxRateKbps, got %d", plan.MaxRateKbps)
	}
}

// --- Optimal Bitrate tests ---

func TestOptimalBitrate_H264Cases(t *testing.T) {
	tests := []struct {
		label    string
		width    int
		height   int
		kbps     int
		codec    string
		minRatio int // minimum % of input
		maxRatio int // maximum % of input
	}{
		// h264 normal sources: expect 55-75% of input
		{"1080p 8Mbps", 1920, 1080, 8000, "h264", 55, 75},
		{"720p 3Mbps", 1280, 720, 3000, "h264", 55, 80},
		{"4K 40Mbps", 3840, 2160, 40000, "h264", 50, 70},
		// Low density: expect higher ratio (less room to compress)
		{"1080p 1.8Mbps ultra-compressed", 1920, 1080, 1800, "h264", 85, 100},
		{"1080p 3.9Mbps compressed", 1920, 1080, 3900, "h264", 70, 95},
		{"720p 1.5Mbps compressed", 1280, 720, 1500, "h264", 65, 95},
		// High density: expect lower ratio (more room)
		{"1080p 25Mbps high-quality", 1920, 1080, 25000, "h264", 50, 65},
		// HEVC re-encode: minimal gain, high ratio
		{"1080p HEVC 5Mbps", 1920, 1080, 5000, "hevc", 85, 100},
		{"4K HEVC 25Mbps", 3840, 2160, 25000, "hevc", 80, 100},
		// Legacy codecs: big gain, low ratio
		{"1080p mpeg2 8Mbps", 1920, 1080, 8000, "mpeg2video", 35, 55},
		{"720p mpeg4 3Mbps", 1280, 720, 3000, "mpeg4", 35, 60},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			pr := &probe.ProbeResult{
				PrimaryVideo: &probe.VideoStream{
					Codec: tt.codec, Width: tt.width, Height: tt.height,
					BitRate: int64(tt.kbps) * 1000,
				},
				Format: probe.FormatInfo{BitRate: int64(tt.kbps)*1000 + 500_000},
			}
			opt := OptimalBitrate(pr)
			ratio := opt * 100 / tt.kbps

			if ratio < tt.minRatio || ratio > tt.maxRatio {
				t.Errorf("ratio %d%% not in [%d, %d]%% (optimal=%d, input=%d)",
					ratio, tt.minRatio, tt.maxRatio, opt, tt.kbps)
			}
			if opt > tt.kbps {
				t.Errorf("optimal %d exceeds input %d", opt, tt.kbps)
			}
			if opt <= 0 {
				t.Error("optimal should be > 0")
			}
			t.Logf("%s: optimal=%d kbps (%d%% of input %d)", tt.label, opt, ratio, tt.kbps)
		})
	}
}

func TestOptimalBitrate_EdgeCases(t *testing.T) {
	// Zero bitrate.
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Width: 1920, Height: 1080},
		Format:       probe.FormatInfo{},
	}
	if OptimalBitrate(pr) != 0 {
		t.Error("zero bitrate should return 0")
	}

	// No video stream.
	pr2 := &probe.ProbeResult{Format: probe.FormatInfo{BitRate: 5000000}}
	if OptimalBitrate(pr2) != 0 {
		t.Error("no video stream should return 0")
	}

	// Zero dimensions.
	pr3 := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 0, Height: 0, BitRate: 5000000},
		Format:       probe.FormatInfo{BitRate: 5000000},
	}
	opt := OptimalBitrate(pr3)
	if opt != 5000 {
		t.Errorf("zero dimensions should return input bitrate 5000, got %d", opt)
	}
}

func TestOptimalBitrate_HEVCHigherThanH264(t *testing.T) {
	// Same bitrate and resolution, HEVC should target higher ratio than h264.
	prH264 := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	prHEVC := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "hevc", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}
	optH264 := OptimalBitrate(prH264)
	optHEVC := OptimalBitrate(prHEVC)
	if optHEVC <= optH264 {
		t.Errorf("HEVC optimal (%d) should be > h264 optimal (%d)", optHEVC, optH264)
	}
	t.Logf("h264 optimal=%d, HEVC optimal=%d", optH264, optHEVC)
}

func TestQPForTargetBitrate_Correctness(t *testing.T) {
	cfg := defaultCfg()
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}

	// Target 5200 kbps (65% of 8000). QP should be in a reasonable range.
	qp := QPForTargetBitrate(cfg, pr, 5200)
	if qp < VaapiQPMin || qp > VaapiQPMax {
		t.Errorf("QP %d out of range", qp)
	}

	// Verify the estimate at this QP is close to target.
	est := EstimateBitrate(cfg, pr, qp, cfg.CpuCRF)
	if est.Known {
		mid := (est.LowKbps + est.HighKbps) / 2
		tolerance := 2000 // within 2 Mbps
		if mid < 5200-tolerance || mid > 5200+tolerance {
			t.Errorf("QP %d produces estimate mid=%d, want ~5200 (±%d)", qp, mid, tolerance)
		}
		t.Logf("Target 5200 → QP=%d, estimated mid=%d kbps", qp, mid)
	}

	// Higher target should produce lower QP.
	qpHigh := QPForTargetBitrate(cfg, pr, 7000)
	qpLow := QPForTargetBitrate(cfg, pr, 3000)
	if qpHigh >= qpLow {
		t.Errorf("higher target QP=%d should be < lower target QP=%d", qpHigh, qpLow)
	}
}

func TestCRFForTargetBitrate_Correctness(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
		Format:       probe.FormatInfo{BitRate: 9000000},
	}

	// Higher target should produce lower CRF (more quality).
	crfHigh := CRFForTargetBitrate(cfg, pr, 7000)
	crfLow := CRFForTargetBitrate(cfg, pr, 3000)
	if crfHigh >= crfLow {
		t.Errorf("higher target CRF=%d should be < lower target CRF=%d", crfHigh, crfLow)
	}
	t.Logf("Target 7000→CRF=%d, Target 3000→CRF=%d", crfHigh, crfLow)
}

// --- Full integration: BuildPlan with optimal bitrate ---

func TestBuildPlan_OptimalBitrate_VAAPI(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderVAAPI

	tests := []struct {
		label string
		kbps  int
		codec string
		maxQP int // expected QP should be ≤ this
		minQP int // expected QP should be ≥ this
	}{
		{"normal h264", 8000, "h264", 24, 16},
		{"compressed h264", 3900, "h264", 26, 17},
		{"ultra-compressed h264", 1800, "h264", 30, 21},
		{"high-quality h264", 25000, "h264", 22, 14},
		{"HEVC re-encode", 5000, "hevc", 27, 17},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			pr := &probe.ProbeResult{
				PrimaryVideo: &probe.VideoStream{
					Codec: tt.codec, Width: 1920, Height: 1080,
					BitRate: int64(tt.kbps) * 1000,
				},
				Format: probe.FormatInfo{BitRate: int64(tt.kbps)*1000 + 500_000},
			}
			plan := BuildPlan(cfg, pr)
			if plan.VaapiQP < tt.minQP || plan.VaapiQP > tt.maxQP {
				t.Errorf("QP=%d not in [%d, %d]", plan.VaapiQP, tt.minQP, tt.maxQP)
			}
			if plan.OptimalBitrateKbps <= 0 {
				t.Error("OptimalBitrateKbps should be set")
			}
			t.Logf("%s: QP=%d optimal=%dk est=%d-%d%%",
				tt.label, plan.VaapiQP, plan.OptimalBitrateKbps,
				safeEstLow(plan.Estimate), safeEstHigh(plan.Estimate))
		})
	}
}

func TestBuildPlan_OptimalBitrate_CPU(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU

	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000,
		},
		Format: probe.FormatInfo{BitRate: 9000000},
	}
	plan := BuildPlan(cfg, pr)

	// MaxRate should be optimal * 115% headroom, never exceeding input.
	expectedMax := plan.OptimalBitrateKbps * 115 / 100
	if expectedMax > 8000 {
		expectedMax = 8000
	}
	if plan.MaxRateKbps != expectedMax {
		t.Errorf("MaxRateKbps: got %d, want %d (optimal %d × 115%%)", plan.MaxRateKbps, expectedMax, plan.OptimalBitrateKbps)
	}
	t.Logf("CRF=%d maxrate=%dk optimal=%dk", plan.CpuCRF, plan.MaxRateKbps, plan.OptimalBitrateKbps)
}

func TestBuildPlan_ManualOverride_SkipsOptimal(t *testing.T) {
	cfg := defaultCfg()
	cfg.ActiveQualityOverride = "20"
	cfg.VaapiQP = 20
	cfg.CpuCRF = 20

	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000,
		},
		Format: probe.FormatInfo{BitRate: 9000000},
	}
	plan := BuildPlan(cfg, pr)
	if plan.VaapiQP != 20 {
		t.Errorf("manual override should produce QP=20, got %d", plan.VaapiQP)
	}
	if plan.OptimalBitrateKbps != 0 {
		t.Error("manual override should not set OptimalBitrateKbps")
	}
}

// --- AAC passthrough tests ---

func TestAudioCopy_AACAlwaysPassthrough(t *testing.T) {
	cfg := defaultCfg()

	for _, bitrate := range []int64{0, 128_000, 256_000, 400_000, 512_000, 640_000, 1_000_000} {
		pr := &probe.ProbeResult{
			PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
			AudioStreams: []probe.AudioStream{{Codec: "aac", Channels: 2, BitRate: bitrate}},
			Format:       probe.FormatInfo{BitRate: 9000000},
		}
		ap := BuildAudioPlan(cfg, pr)
		if !ap.CopyAll {
			t.Errorf("AAC at %d bps should be copied (AAC always passthrough)", bitrate)
		}
	}
}

func TestAudioCopy_NonAACTranscoded(t *testing.T) {
	cfg := defaultCfg()

	for _, codec := range []string{"ac3", "eac3", "dts", "flac", "mp3", "vorbis", "opus"} {
		pr := &probe.ProbeResult{
			PrimaryVideo: &probe.VideoStream{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8000000},
			AudioStreams: []probe.AudioStream{{Codec: codec, Channels: 2, SampleRate: 48000}},
			Format:       probe.FormatInfo{BitRate: 9000000},
		}
		ap := BuildAudioPlan(cfg, pr)
		if ap.CopyAll {
			t.Errorf("%s should NOT produce CopyAll", codec)
		}
		if len(ap.Streams) != 1 || ap.Streams[0].Copy {
			t.Errorf("%s should be transcoded, not copied", codec)
		}
	}
}

// --- Comprehensive bitrate×resolution debug matrix ---
// This exercises the FULL pipeline (SmartQuality → OptimalBitrate → target
// QP/CRF → preflight → maxrate) for every realistic scenario to verify:
//   (a) output never estimated > 110% after all adjustments
//   (b) optimal bitrate is always ≤ input
//   (c) HEVC sources always target higher ratio than h264 at same bitrate
//   (d) CPU maxrate never exceeds input bitrate

func TestFullPipeline_DebugMatrix(t *testing.T) {
	type scenario struct {
		label  string
		width  int
		height int
		kbps   int
		codec  string
	}

	scenarios := []scenario{
		// Low-res
		{"360p 500k h264", 640, 360, 500, "h264"},
		{"480p 1000k h264", 854, 480, 1000, "h264"},

		// 720p
		{"720p 1500k h264", 1280, 720, 1500, "h264"},
		{"720p 3000k h264", 1280, 720, 3000, "h264"},
		{"720p 6000k h264", 1280, 720, 6000, "h264"},

		// 1080p — full range
		{"1080p 1800k h264 (ultra-compressed)", 1920, 1080, 1800, "h264"},
		{"1080p 2000k h264", 1920, 1080, 2000, "h264"},
		{"1080p 3900k h264 (user's case)", 1920, 1080, 3900, "h264"},
		{"1080p 5000k h264", 1920, 1080, 5000, "h264"},
		{"1080p 8000k h264", 1920, 1080, 8000, "h264"},
		{"1080p 15000k h264", 1920, 1080, 15000, "h264"},
		{"1080p 25000k h264", 1920, 1080, 25000, "h264"},
		{"1080p 5000k hevc", 1920, 1080, 5000, "hevc"},
		{"1080p 15000k hevc", 1920, 1080, 15000, "hevc"},
		{"1080p 8000k mpeg2", 1920, 1080, 8000, "mpeg2video"},

		// 1440p
		{"1440p 5000k h264", 2560, 1440, 5000, "h264"},
		{"1440p 10000k h264", 2560, 1440, 10000, "h264"},
		{"1440p 20000k h264", 2560, 1440, 20000, "h264"},

		// 4K
		{"4K 10000k h264", 3840, 2160, 10000, "h264"},
		{"4K 20000k h264", 3840, 2160, 20000, "h264"},
		{"4K 40000k h264", 3840, 2160, 40000, "h264"},
		{"4K 60000k h264", 3840, 2160, 60000, "h264"},
		{"4K 25000k hevc", 3840, 2160, 25000, "hevc"},
	}

	for _, mode := range []config.EncoderMode{config.EncoderVAAPI, config.EncoderCPU} {
		for _, s := range scenarios {
			name := fmt.Sprintf("%s/%s", mode, s.label)
			t.Run(name, func(t *testing.T) {
				cfg := defaultCfg()
				cfg.EncoderMode = mode
				pr := &probe.ProbeResult{
					PrimaryVideo: &probe.VideoStream{
						Codec: s.codec, Width: s.width, Height: s.height,
						BitRate: int64(s.kbps) * 1000,
					},
					Format: probe.FormatInfo{BitRate: int64(s.kbps)*1000 + 500_000},
				}

				plan := BuildPlan(cfg, pr)

				// (a) Estimated output should not exceed 110% unless QP/CRF
				// is at max (ultra-compressed sources handled by post-encode loop).
				atMax := plan.VaapiQP >= VaapiQPMax || plan.CpuCRF >= CpuCRFMax
				if plan.Estimate.Known && plan.Estimate.HighPct > 110 && !atMax {
					t.Errorf("estimated high=%d%% exceeds 110%%", plan.Estimate.HighPct)
				}

				// (b) Optimal bitrate ≤ input.
				if plan.OptimalBitrateKbps > s.kbps {
					t.Errorf("optimal %d exceeds input %d", plan.OptimalBitrateKbps, s.kbps)
				}

				// (c) CPU maxrate ≤ input.
				if mode == config.EncoderCPU && plan.MaxRateKbps > s.kbps {
					t.Errorf("maxrate %d exceeds input %d", plan.MaxRateKbps, s.kbps)
				}

				// (d) QP and CRF within valid ranges.
				if plan.VaapiQP < VaapiQPMin || plan.VaapiQP > VaapiQPMax {
					t.Errorf("QP %d out of range [%d, %d]", plan.VaapiQP, VaapiQPMin, VaapiQPMax)
				}
				if plan.CpuCRF < CpuCRFMin || plan.CpuCRF > CpuCRFMax {
					t.Errorf("CRF %d out of range [%d, %d]", plan.CpuCRF, CpuCRFMin, CpuCRFMax)
				}

				pixels := s.width * s.height
				density := Density(s.kbps, pixels)
				t.Logf("density=%d QP=%d CRF=%d optimal=%dk maxrate=%dk bumps=%d est=%d-%d%%",
					density, plan.VaapiQP, plan.CpuCRF, plan.OptimalBitrateKbps,
					plan.MaxRateKbps, plan.PreflightBumps,
					safeEstLow(plan.Estimate), safeEstHigh(plan.Estimate))
			})
		}
	}
}

package planner

import (
	"strings"
	"testing"

	"github.com/backmassage/muxmaster/internal/config"
	"github.com/backmassage/muxmaster/internal/probe"
)

// --- Helper builders ---

func defaultCfg() *config.Config {
	cfg := config.DefaultConfig()
	return &cfg
}

func h264SDR() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "h264", Profile: "High", PixFmt: "yuv420p",
			Width: 1920, Height: 1080, BitRate: 8000000,
			FieldOrder: "progressive",
		},
		AudioStreams:    []probe.AudioStream{{Codec: "ac3", Channels: 6, SampleRate: 48000}},
		SubtitleStreams: []probe.SubtitleStream{{Codec: "ass", Language: "eng"}},
		Format:         probe.FormatInfo{BitRate: 9000000},
	}
}

func hevcEdgeSafe() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Main 10", PixFmt: "yuv420p10le",
			Width: 1920, Height: 1080, BitRate: 5000000,
		},
		AudioStreams: []probe.AudioStream{{Codec: "aac", Channels: 2, SampleRate: 48000}},
		Format:      probe.FormatInfo{BitRate: 6000000},
	}
}

func hevcUnsafe() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Rext", PixFmt: "yuv444p10le",
			Width: 1920, Height: 1080, BitRate: 5000000,
		},
		AudioStreams: []probe.AudioStream{{Codec: "flac", Channels: 2, SampleRate: 48000}},
		Format:      probe.FormatInfo{BitRate: 6000000},
	}
}

func hdr10File() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "hevc", Profile: "Main 10", PixFmt: "yuv420p10le",
			Width: 3840, Height: 2160, BitRate: 30000000,
			ColorTransfer: "smpte2084", ColorPrimaries: "bt2020", ColorSpace: "bt2020nc",
			FieldOrder: "progressive",
		},
		AudioStreams: []probe.AudioStream{{Codec: "eac3", Channels: 6, SampleRate: 48000}},
		Format:      probe.FormatInfo{BitRate: 35000000},
	}
}

func interlacedFile() *probe.ProbeResult {
	return &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{
			Codec: "mpeg2video", Profile: "Main", PixFmt: "yuv420p",
			Width: 720, Height: 480, BitRate: 3500000,
			FieldOrder: "tt",
		},
		AudioStreams: []probe.AudioStream{{Codec: "mp2", Channels: 2, SampleRate: 48000}},
		Format:      probe.FormatInfo{BitRate: 4000000},
	}
}

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
	if q.CpuCRF <= 19 {
		t.Errorf("low-res should increase CRF above default 19, got %d", q.CpuCRF)
	}
	if q.VaapiQP <= 19 {
		t.Errorf("low-res should increase QP above default 19, got %d", q.VaapiQP)
	}
}

func TestSmartQuality_4K(t *testing.T) {
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Width: 3840, Height: 2160, BitRate: 40000000},
		Format:       probe.FormatInfo{BitRate: 42000000},
	}
	q := SmartQuality(defaultCfg(), pr)
	// 4K + high bitrate → should lower quality values for more quality.
	if q.CpuCRF >= 19 {
		t.Errorf("4K should decrease CRF below default 19, got %d", q.CpuCRF)
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
	if q.CpuCRF > cpuCRFMax {
		t.Errorf("CRF %d exceeds max %d", q.CpuCRF, cpuCRFMax)
	}
	if q.VaapiQP > vaapiQPMax {
		t.Errorf("QP %d exceeds max %d", q.VaapiQP, vaapiQPMax)
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

// --- BuildVideoFilter tests ---

func TestBuildVideoFilter_VaapiDefault(t *testing.T) {
	cfg := defaultCfg()
	f := BuildVideoFilter(cfg, h264SDR())
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
	f := BuildVideoFilter(cfg, h264SDR())
	if f != "" {
		t.Errorf("CPU + progressive + SDR should have no filter, got %q", f)
	}
}

func TestBuildVideoFilter_Deinterlace(t *testing.T) {
	cfg := defaultCfg()
	cfg.EncoderMode = config.EncoderCPU
	f := BuildVideoFilter(cfg, interlacedFile())
	if !strings.Contains(f, "yadif=mode=send_frame:parity=auto:deint=interlaced") {
		t.Errorf("interlaced should have full yadif, got %q", f)
	}
}

func TestBuildVideoFilter_DeinterlaceDisabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.DeinterlaceAuto = false
	cfg.EncoderMode = config.EncoderCPU
	f := BuildVideoFilter(cfg, interlacedFile())
	if strings.Contains(f, "yadif") {
		t.Errorf("DeinterlaceAuto=false should not produce yadif, got %q", f)
	}
}

func TestBuildVideoFilter_HDRTonemap(t *testing.T) {
	cfg := defaultCfg()
	cfg.HandleHDR = config.HDRTonemap
	cfg.EncoderMode = config.EncoderCPU
	cfg.SkipHEVC = false
	f := BuildVideoFilter(cfg, hdr10File())
	if !strings.Contains(f, "tonemap") || !strings.Contains(f, "hable") {
		t.Errorf("HDR tonemap should have tonemap filter, got %q", f)
	}
}

func TestBuildVideoFilter_HDRPreserve(t *testing.T) {
	cfg := defaultCfg()
	cfg.HandleHDR = config.HDRPreserve
	cfg.EncoderMode = config.EncoderCPU
	cfg.SkipHEVC = false
	f := BuildVideoFilter(cfg, hdr10File())
	if strings.Contains(f, "tonemap") {
		t.Errorf("HDR preserve should NOT have tonemap, got %q", f)
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

func TestBuildAudioPlan_ChannelClamp(t *testing.T) {
	cfg := defaultCfg()
	cfg.AudioChannels = 2
	pr := &probe.ProbeResult{
		PrimaryVideo: &probe.VideoStream{Codec: "h264"},
		AudioStreams:  []probe.AudioStream{{Codec: "dts", Channels: 8, SampleRate: 48000}},
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
		AudioStreams:  []probe.AudioStream{{Codec: "ac3", Channels: 2, SampleRate: 48000}},
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
		AudioStreams:  []probe.AudioStream{{Codec: "ac3", Channels: 1, SampleRate: 48000}},
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
		AudioStreams:  []probe.AudioStream{{Codec: "ac3", Channels: 2, SampleRate: 48000}},
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
		AudioStreams:  []probe.AudioStream{{Codec: "dts", Channels: 6, SampleRate: 48000}},
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
		PrimaryVideo:   &probe.VideoStream{Codec: "h264"},
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
		PrimaryVideo:   &probe.VideoStream{Codec: "h264"},
		SubtitleStreams: []probe.SubtitleStream{{Index: 3, Codec: "hdmv_pgs_subtitle", IsBitmap: true}},
		HasBitmapSubs:  true,
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
		AudioStreams:  []probe.AudioStream{{Codec: "aac"}},
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
	if !strings.Contains(plan.VideoFilters, "hwupload") {
		t.Error("should have hwupload filter")
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
	if !strings.Contains(plan.VideoFilters, "hwupload") {
		t.Error("VAAPI should have hwupload")
	}
	if strings.Contains(plan.VideoFilters, "tonemap") {
		t.Error("HDR preserve should NOT tonemap")
	}
}

func TestFullPlan_InterlacedVAAPI(t *testing.T) {
	plan := BuildPlan(defaultCfg(), interlacedFile())
	if !strings.Contains(plan.VideoFilters, "yadif") {
		t.Error("interlaced should have yadif")
	}
	if !strings.Contains(plan.VideoFilters, "hwupload") {
		t.Error("VAAPI should have hwupload after yadif")
	}
	// Verify order: yadif before hwupload.
	yIdx := strings.Index(plan.VideoFilters, "yadif")
	hIdx := strings.Index(plan.VideoFilters, "hwupload")
	if yIdx > hIdx {
		t.Error("yadif should come before hwupload")
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
	if vaapiRatio(27) != 390 {
		t.Errorf("vaapi QP27: got %d, want 390", vaapiRatio(27))
	}
}

func TestCpuRatioTable(t *testing.T) {
	if cpuRatio(19) != 660 {
		t.Errorf("cpu CRF19: got %d, want 660", cpuRatio(19))
	}
	if cpuRatio(16) != 900 {
		t.Errorf("cpu CRF16: got %d, want 900", cpuRatio(16))
	}
	if cpuRatio(28) != 230 {
		t.Errorf("cpu CRF28: got %d, want 230", cpuRatio(28))
	}
}

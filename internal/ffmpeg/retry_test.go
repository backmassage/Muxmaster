package ffmpeg

import (
	"testing"

	"github.com/backmassage/muxmaster/internal/planner"
)

func testPlan() *planner.FilePlan {
	return &planner.FilePlan{
		VaapiQP:       22,
		CpuCRF:        22,
		MuxQueueSize:  4096,
		IncludeSubs:   true,
		IncludeAttach: true,
	}
}

func TestNewRetryState_InitialValues(t *testing.T) {
	plan := testPlan()
	rs := NewRetryState(plan)
	if rs.Attempt != 0 {
		t.Errorf("Attempt: got %d, want 0", rs.Attempt)
	}
	if rs.VaapiQP != 22 {
		t.Errorf("VaapiQP: got %d, want 22", rs.VaapiQP)
	}
	if rs.CpuCRF != 22 {
		t.Errorf("CpuCRF: got %d, want 22", rs.CpuCRF)
	}
	if !rs.IncludeSubs {
		t.Error("IncludeSubs should be true")
	}
	if !rs.IncludeAttach {
		t.Error("IncludeAttach should be true")
	}
	if rs.MuxQueueSize != 4096 {
		t.Errorf("MuxQueueSize: got %d, want 4096", rs.MuxQueueSize)
	}
}

func TestAdvance_DropAttachments(t *testing.T) {
	rs := NewRetryState(testPlan())
	action := rs.Advance("Attachment stream 3 has no filename tag")
	if action != RetryDropAttach {
		t.Errorf("expected RetryDropAttach, got %d", action)
	}
	if rs.IncludeAttach {
		t.Error("IncludeAttach should be false after drop")
	}
}

func TestAdvance_DropSubs(t *testing.T) {
	rs := NewRetryState(testPlan())
	rs.IncludeAttach = false
	action := rs.Advance("Subtitle codec mov_text is not supported")
	if action != RetryDropSubs {
		t.Errorf("expected RetryDropSubs, got %d", action)
	}
	if rs.IncludeSubs {
		t.Error("IncludeSubs should be false after drop")
	}
}

func TestAdvance_RespectsMaxAttempts(t *testing.T) {
	rs := NewRetryState(testPlan())
	for i := 0; i < maxAttempts; i++ {
		rs.Advance("Subtitle codec mov_text is not supported")
	}
	action := rs.Advance("any error")
	if action != RetryNone {
		t.Error("should return RetryNone after max attempts")
	}
}

func TestAdvance_MuxQueueEscalation(t *testing.T) {
	rs := NewRetryState(testPlan())
	rs.IncludeAttach = false
	rs.IncludeSubs = false
	action := rs.Advance("Too many packets buffered for output stream #0:1")
	if action != RetryIncreaseMux {
		t.Errorf("expected RetryIncreaseMux, got %d", action)
	}
	if rs.MuxQueueSize != muxQueueEscalate {
		t.Errorf("MuxQueueSize: got %d, want %d", rs.MuxQueueSize, muxQueueEscalate)
	}
}

func TestSubtitleIssue_DoesNotMatchGenericEncoder(t *testing.T) {
	cases := []string{
		"Unknown encoder 'hevc_vaapi'",
		"Codec h264 is not supported in this configuration",
		"[enc:hevc_vaapi] Could not open encoder before EOF",
	}
	for _, stderr := range cases {
		if MatchSubtitleIssue(stderr) {
			t.Errorf("MatchSubtitleIssue should not match non-subtitle error: %q", stderr)
		}
	}
}

func TestSubtitleIssue_MatchesSubtitleErrors(t *testing.T) {
	cases := []string{
		"Subtitle codec mov_text is not supported",
		"Could not find tag for codec ass in stream #0:3 (subtitle)",
		"Error initializing output stream #0:2 -- subtitle",
		"Error while opening encoder for output stream #0:2 -- subtitle codec",
		"Subtitle encoding currently only possible from text to text or bitmap to bitmap",
	}
	for _, stderr := range cases {
		if !MatchSubtitleIssue(stderr) {
			t.Errorf("MatchSubtitleIssue should match subtitle error: %q", stderr)
		}
	}
}

func TestTimestampIssue_Matches(t *testing.T) {
	cases := []string{
		"Non-monotonous DTS in output stream",
		"non monotonically increasing dts in output",
		"invalid, non monotonically increasing dts",
		"DTS 12345 out of order",
		"PTS 12345 out of order",
		"pts has no value",
		"missing PTS",
		"Timestamps are unset",
	}
	for _, stderr := range cases {
		if !MatchTimestampIssue(stderr) {
			t.Errorf("MatchTimestampIssue should match: %q", stderr)
		}
	}
}

func TestAdvance_TimestampFix(t *testing.T) {
	rs := NewRetryState(testPlan())
	rs.IncludeAttach = false
	rs.IncludeSubs = false
	action := rs.Advance("Non-monotonous DTS in output stream")
	if action != RetryFixTimestamps {
		t.Errorf("expected RetryFixTimestamps, got %d", action)
	}
	if !rs.TimestampFix {
		t.Error("TimestampFix should be true")
	}
}

func TestHWDecodeIssue_Matches(t *testing.T) {
	cases := []string{
		"Failed setup for format vaapi: hwaccel initialisation returned error.",
		"hwaccel initialisation returned error",
		"Impossible to convert between the formats supported by the filter 'Parsed_scale_vaapi_0' and the filter 'auto_scaler_0'",
		"No usable encoding profile found.",
	}
	for _, stderr := range cases {
		if !MatchHWDecodeIssue(stderr) {
			t.Errorf("MatchHWDecodeIssue should match: %q", stderr)
		}
	}
}

func TestAdvance_DisableHWDecode(t *testing.T) {
	plan := testPlan()
	plan.HWDecode = true
	rs := NewRetryState(plan)
	if !rs.HWDecode {
		t.Fatal("HWDecode should be seeded from plan")
	}
	action := rs.Advance("Failed setup for format vaapi: hwaccel initialisation returned error.")
	if action != RetryDisableHWDecode {
		t.Errorf("expected RetryDisableHWDecode, got %d", action)
	}
	if rs.HWDecode {
		t.Error("HWDecode should be false after fallback")
	}
}

func TestAdvance_NoHWDecodeRetryForSoftwarePlans(t *testing.T) {
	rs := NewRetryState(testPlan()) // plan.HWDecode = false
	action := rs.Advance("No usable encoding profile found.")
	if action != RetryNone {
		t.Errorf("software-decode plan should not take HW decode retry, got %d", action)
	}
}

func TestRateControlIssue_Matches(t *testing.T) {
	cases := []string{
		"[hevc_vaapi @ 0x55] Driver does not support QVBR RC mode (supported modes: CQP, CBR, VBR).",
		"RC mode QVBR is not supported by the driver",
		"Invalid rate control mode",
		"Unsupported rate control mode requested",
	}
	for _, stderr := range cases {
		if !MatchRateControlIssue(stderr) {
			t.Errorf("MatchRateControlIssue should match: %q", stderr)
		}
	}
	if MatchRateControlIssue("Error while opening encoder for output stream") {
		t.Error("MatchRateControlIssue should not match generic encoder error")
	}
}

func TestBFrameIssue_Matches(t *testing.T) {
	cases := []string{
		"[hevc_vaapi @ 0x55] Driver does not support B-frames in this configuration",
		"B-frames are not supported by this driver",
		"invalid number of B-frames",
	}
	for _, stderr := range cases {
		if !MatchBFrameIssue(stderr) {
			t.Errorf("MatchBFrameIssue should match: %q", stderr)
		}
	}
}

func TestAdvance_DisableQVBR(t *testing.T) {
	plan := testPlan()
	plan.VaapiQVBR = true
	rs := NewRetryState(plan)
	if !rs.VaapiQVBR {
		t.Fatal("VaapiQVBR should be seeded from plan")
	}
	action := rs.Advance("Driver does not support QVBR RC mode (supported modes: CQP).")
	if action != RetryDisableQVBR {
		t.Errorf("expected RetryDisableQVBR, got %d", action)
	}
	if rs.VaapiQVBR {
		t.Error("VaapiQVBR should be false after fallback")
	}
}

func TestAdvance_NoQVBRRetryForCQPPlans(t *testing.T) {
	rs := NewRetryState(testPlan()) // plan.VaapiQVBR = false
	action := rs.Advance("Driver does not support QVBR RC mode (supported modes: CQP).")
	if action != RetryNone {
		t.Errorf("CQP plan should not take QVBR retry, got %d", action)
	}
}

func TestAdvance_DropBFrames(t *testing.T) {
	plan := testPlan()
	plan.VaapiBFrames = true
	rs := NewRetryState(plan)
	action := rs.Advance("Driver does not support B-frames")
	if action != RetryDropBFrames {
		t.Errorf("expected RetryDropBFrames, got %d", action)
	}
	if rs.VaapiBFrames {
		t.Error("VaapiBFrames should be false after fallback")
	}
}

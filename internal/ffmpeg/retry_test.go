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

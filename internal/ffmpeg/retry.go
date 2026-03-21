// retry.go implements the retry state machine for recoverable ffmpeg failures.
package ffmpeg

import "github.com/backmassage/muxmaster/internal/planner"

// RetryAction identifies which fix was applied (or none).
type RetryAction int

const (
	RetryNone          RetryAction = iota
	RetryDropAttach                // Remove attachment streams.
	RetryDropSubs                  // Remove subtitle streams.
	RetryIncreaseMux               // Raise max_muxing_queue_size to 16384.
	RetryFixTimestamps             // Enable +genpts+discardcorrupt.
)

const (
	maxAttempts      = 4
	muxQueueDefault  = 4096
	muxQueueEscalate = 16384
)

// RetryState tracks which fallback fixes have been applied across ffmpeg
// retry attempts for a single file. Quality adjustment is handled at plan
// time (preflight QP adjustment and CPU maxrate cap), not via retries.
type RetryState struct {
	Attempt     int
	MaxAttempts int

	IncludeAttach bool
	IncludeSubs   bool
	MuxQueueSize  int
	TimestampFix  bool

	VaapiQP int
	CpuCRF  int
}

// NewRetryState initializes a RetryState from the plan's initial values.
func NewRetryState(plan *planner.FilePlan) *RetryState {
	return &RetryState{
		Attempt:       0,
		MaxAttempts:   maxAttempts,
		IncludeAttach: plan.IncludeAttach,
		IncludeSubs:   plan.IncludeSubs,
		MuxQueueSize:  plan.MuxQueueSize,
		TimestampFix:  plan.TimestampFix,
		VaapiQP:       plan.VaapiQP,
		CpuCRF:        plan.CpuCRF,
	}
}

// Advance inspects stderr from a failed ffmpeg run, finds the first matching
// error pattern whose fix has not yet been applied, applies that fix, and
// returns the action taken. Returns RetryNone when no fixable pattern matches
// or the attempt limit is reached.
//
// Pattern evaluation order: attachment → subtitle → mux queue → timestamp.
// Only one fix is applied per call (one fix per retry attempt).
func (s *RetryState) Advance(stderr string) RetryAction {
	s.Attempt++
	if s.Attempt >= s.MaxAttempts {
		return RetryNone
	}

	if s.IncludeAttach && MatchAttachmentIssue(stderr) {
		s.IncludeAttach = false
		return RetryDropAttach
	}
	if s.IncludeSubs && MatchSubtitleIssue(stderr) {
		s.IncludeSubs = false
		return RetryDropSubs
	}
	if s.MuxQueueSize < muxQueueEscalate && MatchMuxQueueOverflow(stderr) {
		s.MuxQueueSize = muxQueueEscalate
		return RetryIncreaseMux
	}
	if !s.TimestampFix && MatchTimestampIssue(stderr) {
		s.TimestampFix = true
		return RetryFixTimestamps
	}

	return RetryNone
}

// errors.go classifies ffmpeg stderr output using regex-based patterns.
package ffmpeg

import "regexp"

// Pre-compiled regexes for classifying ffmpeg stderr output into retryable
// error categories. Checked in order by [RetryState.Advance]; the first
// matching pattern whose fix has not yet been applied wins.
var (
	reAttachmentIssue = regexp.MustCompile(
		`Attachment stream \d+ has no (filename|mimetype) tag`)

	reSubtitleIssue = regexp.MustCompile(
		`(?i)Subtitle codec .* is not supported|` +
			`Could not find tag for codec .* in stream .*subtitle|` +
			`Error initializing output stream .*subtitle|` +
			`Error while opening encoder for output stream .*subtitle|` +
			`Subtitle encoding currently only possible from text to text or bitmap to bitmap`)

	reMuxQueueOverflow = regexp.MustCompile(
		`Too many packets buffered for output stream`)

	// "Non-monoton(ous|ic) DTS" spans ffmpeg versions: pre-5.1 emitted
	// "Non-monotonous DTS", n5.1+ (incl. n8.1 / lavf 62) emits the corrected
	// "Non-monotonic DTS". "Non-increasing DTS in stream" is the lavf 62
	// interleave wording. Missing the modern spelling sent fixable remux
	// timestamp failures (e.g. eac3-in-MP4 → MKV) to "no applicable retry".
	reTimestampIssue = regexp.MustCompile(
		`(?i)Non-monoton(ous|ic) DTS|Non-increasing DTS|` +
			`non monotonically increasing dts|` +
			`invalid, non monotonically increasing dts|` +
			`DTS .*out of order|PTS .*out of order|` +
			`pts has no value|missing PTS|Timestamps are unset`)

	// Hardware decode failures: the driver can't decode this codec/profile
	// (e.g. VC-1 or 4:2:2 H.264 on many stacks). When -hwaccel init fails,
	// ffmpeg falls back to software frames, which then can't enter the
	// VAAPI filter graph ("Impossible to convert between the formats").
	reHWDecodeIssue = regexp.MustCompile(
		`(?i)Failed setup for format vaapi|` +
			`hwaccel initialisation returned error|` +
			`Impossible to convert between the formats supported by the filter|` +
			`No usable encoding profile found`)

	// Rate control init failures: vaapi_encode validates the requested
	// -rc_mode against VAConfigAttribRateControl at init and errors out
	// when the driver lacks the mode (QVBR needs Mesa >= 24.3 on AMD).
	reRateControlIssue = regexp.MustCompile(
		`(?i)does not support .*RC mode|` +
			`RC mode.{0,30}not supported|` +
			`Invalid rate control mode|` +
			`unsupported rate control`)

	// B-frame failures: drivers without HEVC B-frame encode support
	// (AMD VCN <= 4) reject -bf at init on some stacks.
	reBFrameIssue = regexp.MustCompile(
		`(?i)does not support B-(frames|pictures)|` +
			`B-?(frames|pictures).{0,30}not supported|` +
			`invalid number of B-?frames`)
)

// MatchAttachmentIssue reports whether stderr contains an attachment tag error.
func MatchAttachmentIssue(stderr string) bool {
	return reAttachmentIssue.MatchString(stderr)
}

// MatchSubtitleIssue reports whether stderr contains a subtitle muxing error.
func MatchSubtitleIssue(stderr string) bool {
	return reSubtitleIssue.MatchString(stderr)
}

// MatchMuxQueueOverflow reports whether stderr contains a mux queue overflow.
func MatchMuxQueueOverflow(stderr string) bool {
	return reMuxQueueOverflow.MatchString(stderr)
}

// MatchTimestampIssue reports whether stderr contains a timestamp discontinuity.
func MatchTimestampIssue(stderr string) bool {
	return reTimestampIssue.MatchString(stderr)
}

// MatchHWDecodeIssue reports whether stderr contains a hardware decode failure.
func MatchHWDecodeIssue(stderr string) bool {
	return reHWDecodeIssue.MatchString(stderr)
}

// MatchRateControlIssue reports whether stderr contains a VAAPI rate-control
// mode rejection (e.g. QVBR unsupported by the driver).
func MatchRateControlIssue(stderr string) bool {
	return reRateControlIssue.MatchString(stderr)
}

// MatchBFrameIssue reports whether stderr contains a B-frame support failure.
func MatchBFrameIssue(stderr string) bool {
	return reBFrameIssue.MatchString(stderr)
}

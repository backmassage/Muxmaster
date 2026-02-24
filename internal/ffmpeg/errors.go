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
			`Subtitle encoding currently only possible from text to text or bitmap to bitmap|` +
			`Unknown encoder|` +
			`Codec .* is not supported`)

	reMuxQueueOverflow = regexp.MustCompile(
		`Too many packets buffered for output stream`)

	reTimestampIssue = regexp.MustCompile(
		`(?i)Non-monotonous DTS|non monotonically increasing dts|` +
			`invalid, non monotonically increasing dts|` +
			`DTS .*out of order|PTS .*out of order|` +
			`pts has no value|missing PTS|Timestamps are unset`)
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

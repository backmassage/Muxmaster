// ProbeResult, VideoStream, AudioStream, SubtitleStream, FormatInfo types.
package probe

import (
	"strconv"
	"strings"
)

// FormatInfo holds container-level metadata from ffprobe's format section.
type FormatInfo struct {
	Filename       string
	NbStreams      int
	FormatName     string
	FormatLongName string
	Duration       float64
	Size           int64
	BitRate        int64
	Tags           map[string]string
}

// VideoStream holds the parsed properties of a single video stream.
type VideoStream struct {
	Index          int
	Codec          string
	Profile        string
	PixFmt         string
	Width          int
	Height         int
	BitRate        int64
	FieldOrder     string
	ColorTransfer  string
	ColorPrimaries string
	ColorSpace     string
	ChromaLocation string
	IsAttachedPic  bool
	AvgFrameRate   string

	// Dolby Vision (from the DOVI configuration record side data).
	// DoviProfile is 0 when the stream carries no DV metadata; no real-world
	// DV stream uses profile 0, so 0 unambiguously means "not Dolby Vision".
	DoviProfile    int
	DoviBLCompatID int

	MasteringDisplay  *MasteringDisplay
	ContentLightLevel *ContentLightLevel
}

// MasteringDisplay holds SMPTE ST.2086 mastering display color volume metadata.
// Chromaticity values are CIE 1931 xy coordinates scaled by 50000.
// Luminance values are in units of 1/10000 cd/m².
type MasteringDisplay struct {
	RedX, RedY     int
	GreenX, GreenY int
	BlueX, BlueY   int
	WhiteX, WhiteY int
	MinLuminance   int
	MaxLuminance   int
}

// ContentLightLevel holds MaxCLL and MaxFALL in nits.
type ContentLightLevel struct {
	MaxCLL  int
	MaxFALL int
}

// AudioStream holds the parsed properties of a single audio stream.
type AudioStream struct {
	Index         int
	Codec         string
	Channels      int
	ChannelLayout string
	SampleRate    int
	BitRate       int64
	Language      string
	IsDefault     bool
}

// SubtitleStream holds the parsed properties of a single subtitle stream.
type SubtitleStream struct {
	Index    int
	Codec    string
	Language string
	IsBitmap bool
}

// FrameRate returns the stream's average frame rate in frames per second,
// parsed from ffprobe's rational avg_frame_rate (e.g. "24000/1001").
// Returns 0 when the value is missing, malformed, or degenerate ("0/0").
func (v *VideoStream) FrameRate() float64 {
	s := v.AvgFrameRate
	if idx := strings.IndexByte(s, '/'); idx >= 0 {
		num, err1 := strconv.ParseFloat(s[:idx], 64)
		den, err2 := strconv.ParseFloat(s[idx+1:], 64)
		if err1 != nil || err2 != nil || den == 0 {
			return 0
		}
		return num / den
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// ProbeResult is the fully parsed output of a single ffprobe JSON call.
// PrimaryVideo is the first non-attached-pic video stream (nil if none).
// AttachedPicIdxs are the absolute stream indices of cover-art video
// streams (attached_pic disposition).
type ProbeResult struct {
	Format          FormatInfo
	PrimaryVideo    *VideoStream
	AudioStreams    []AudioStream
	SubtitleStreams []SubtitleStream
	AttachedPicIdxs []int
	HasBitmapSubs   bool
}

// VideoBitRate returns the primary video stream bitrate in bits/sec,
// falling back to the format-level bitrate (minus known audio bitrate)
// when the stream value is unavailable or zero.
func (p *ProbeResult) VideoBitRate() int64 {
	if p.PrimaryVideo != nil && p.PrimaryVideo.BitRate > 0 {
		return p.PrimaryVideo.BitRate
	}
	// Format.BitRate is the total container bitrate including all audio.
	// Subtract known audio to approximate the video-only bitrate.
	fb := p.Format.BitRate - p.TotalAudioBitRate()
	if fb > 0 {
		return fb
	}
	// fb <= 0 means audio is reported at or above the whole-container bitrate
	// (corrupt/incomplete metadata) or the format bitrate itself is unknown.
	// Returning the audio-inclusive total would mislabel audio as video, so
	// report 0 ("unknown") and let callers fall back to their defaults.
	return 0
}

// TotalAudioBitRate returns the sum of all known audio stream bitrates
// in bits/sec.
func (p *ProbeResult) TotalAudioBitRate() int64 {
	var total int64
	for _, a := range p.AudioStreams {
		total += a.BitRate
	}
	return total
}

// AudioBitRate returns the first audio stream's bitrate in bits/sec, or 0.
func (p *ProbeResult) AudioBitRate() int64 {
	if len(p.AudioStreams) > 0 && p.AudioStreams[0].BitRate > 0 {
		return p.AudioStreams[0].BitRate
	}
	return 0
}

// Resolution returns "WxH" for the primary video stream, or "unknown".
func (p *ProbeResult) Resolution() string {
	if p.PrimaryVideo == nil || p.PrimaryVideo.Width <= 0 || p.PrimaryVideo.Height <= 0 {
		return "unknown"
	}
	return strconv.Itoa(p.PrimaryVideo.Width) + "x" + strconv.Itoa(p.PrimaryVideo.Height)
}

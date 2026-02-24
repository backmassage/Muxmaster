package probe

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
	IsAttachedPic  bool
	AvgFrameRate   string
}

// AudioStream holds the parsed properties of a single audio stream.
type AudioStream struct {
	Index         int
	Codec         string
	Channels      int
	ChannelLayout string
	SampleRate    int
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

// ProbeResult is the fully parsed output of a single ffprobe JSON call.
// PrimaryVideo is the first non-attached-pic video stream (nil if none).
type ProbeResult struct {
	Format          FormatInfo
	PrimaryVideo    *VideoStream
	AudioStreams    []AudioStream
	SubtitleStreams []SubtitleStream
	HasBitmapSubs   bool
}

// VideoBitRate returns the primary video stream bitrate in bits/sec,
// falling back to the format-level bitrate when the stream value is
// unavailable or zero.
func (p *ProbeResult) VideoBitRate() int64 {
	if p.PrimaryVideo != nil && p.PrimaryVideo.BitRate > 0 {
		return p.PrimaryVideo.BitRate
	}
	return p.Format.BitRate
}

// Resolution returns "WxH" for the primary video stream, or "unknown".
func (p *ProbeResult) Resolution() string {
	if p.PrimaryVideo == nil || p.PrimaryVideo.Width <= 0 || p.PrimaryVideo.Height <= 0 {
		return "unknown"
	}
	return itoa(p.PrimaryVideo.Width) + "x" + itoa(p.PrimaryVideo.Height)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

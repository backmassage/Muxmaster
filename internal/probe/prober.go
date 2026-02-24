package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Probe runs a single ffprobe JSON call against path and returns the
// parsed result. It replaces the ~10 separate ffprobe calls made by the
// legacy shell script.
func Probe(ctx context.Context, path string) (*ProbeResult, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "quiet",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %q: %w", path, err)
	}

	return ParseJSON(out)
}

// ParseJSON converts raw ffprobe JSON output into a ProbeResult.
// Exported for testing without a real ffprobe binary.
func ParseJSON(data []byte) (*ProbeResult, error) {
	var raw ffprobeOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe JSON: %w", err)
	}
	return buildResult(&raw), nil
}

// --- ffprobe JSON wire types ---

type ffprobeOutput struct {
	Format  ffprobeFormat   `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeFormat struct {
	Filename       string            `json:"filename"`
	NbStreams      int               `json:"nb_streams"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	Tags           map[string]string `json:"tags"`
}

type ffprobeStream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecType      string            `json:"codec_type"`
	Profile        string            `json:"profile"`
	PixFmt         string            `json:"pix_fmt"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	BitRate        string            `json:"bit_rate"`
	FieldOrder     string            `json:"field_order"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	ColorSpace     string            `json:"color_space"`
	AvgFrameRate   string            `json:"avg_frame_rate"`
	Channels       int               `json:"channels"`
	ChannelLayout  string            `json:"channel_layout"`
	SampleRate     string            `json:"sample_rate"`
	Disposition    map[string]int    `json:"disposition"`
	Tags           map[string]string `json:"tags"`
}

// --- Conversion from wire types to domain types ---

func buildResult(raw *ffprobeOutput) *ProbeResult {
	pr := &ProbeResult{
		Format: convertFormat(&raw.Format),
	}

	for i := range raw.Streams {
		s := &raw.Streams[i]
		switch s.CodecType {
		case "video":
			vs := convertVideo(s)
			if !vs.IsAttachedPic && pr.PrimaryVideo == nil {
				pr.PrimaryVideo = &vs
			}
		case "audio":
			pr.AudioStreams = append(pr.AudioStreams, convertAudio(s))
		case "subtitle":
			sub := convertSubtitle(s)
			pr.SubtitleStreams = append(pr.SubtitleStreams, sub)
			if sub.IsBitmap {
				pr.HasBitmapSubs = true
			}
		}
	}
	return pr
}

func convertFormat(f *ffprobeFormat) FormatInfo {
	return FormatInfo{
		Filename:       f.Filename,
		NbStreams:      f.NbStreams,
		FormatName:     f.FormatName,
		FormatLongName: f.FormatLongName,
		Duration:       parseFloat(f.Duration),
		Size:           parseInt64(f.Size),
		BitRate:        parseInt64(f.BitRate),
		Tags:           f.Tags,
	}
}

func convertVideo(s *ffprobeStream) VideoStream {
	return VideoStream{
		Index:          s.Index,
		Codec:          s.CodecName,
		Profile:        s.Profile,
		PixFmt:         s.PixFmt,
		Width:          s.Width,
		Height:         s.Height,
		BitRate:        parseInt64(s.BitRate),
		FieldOrder:     s.FieldOrder,
		ColorTransfer:  s.ColorTransfer,
		ColorPrimaries: s.ColorPrimaries,
		ColorSpace:     s.ColorSpace,
		IsAttachedPic:  s.Disposition["attached_pic"] == 1,
		AvgFrameRate:   s.AvgFrameRate,
	}
}

func convertAudio(s *ffprobeStream) AudioStream {
	return AudioStream{
		Index:         s.Index,
		Codec:         s.CodecName,
		Channels:      s.Channels,
		ChannelLayout: s.ChannelLayout,
		SampleRate:    parseInt(s.SampleRate),
		BitRate:       parseInt64(s.BitRate),
		Language:      s.Tags["language"],
		IsDefault:     s.Disposition["default"] == 1,
	}
}

var bitmapSubCodecs = map[string]bool{
	"hdmv_pgs_subtitle": true,
	"dvd_subtitle":      true,
	"dvb_subtitle":      true,
	"xsub":              true,
}

func convertSubtitle(s *ffprobeStream) SubtitleStream {
	return SubtitleStream{
		Index:    s.Index,
		Codec:    s.CodecName,
		Language: s.Tags["language"],
		IsBitmap: bitmapSubCodecs[s.CodecName],
	}
}

// --- Numeric parsing helpers (ffprobe returns numbers as strings) ---

func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	n, _ := strconv.Atoi(s)
	return n
}

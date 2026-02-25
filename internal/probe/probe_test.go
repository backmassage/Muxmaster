package probe

import (
	"testing"
)

// Realistic ffprobe JSON for a Matroska file with:
//   - 1 HEVC Main 10 HDR video stream (1920x1080, smpte2084, bt2020)
//   - 1 AAC stereo audio stream (48000 Hz)
//   - 1 ASS subtitle stream
//   - 1 attached pic (cover art, should be skipped as primary video)
const sampleHDR = `{
  "streams": [
    {
      "index": 0,
      "codec_name": "mjpeg",
      "codec_type": "video",
      "width": 600,
      "height": 900,
      "pix_fmt": "yuvj444p",
      "disposition": { "default": 0, "attached_pic": 1 },
      "tags": { "comment": "Cover (front)" }
    },
    {
      "index": 1,
      "codec_name": "hevc",
      "codec_type": "video",
      "profile": "Main 10",
      "pix_fmt": "yuv420p10le",
      "width": 1920,
      "height": 1080,
      "bit_rate": "5000000",
      "field_order": "progressive",
      "color_transfer": "smpte2084",
      "color_primaries": "bt2020",
      "color_space": "bt2020nc",
      "avg_frame_rate": "24000/1001",
      "disposition": { "default": 1, "attached_pic": 0 },
      "tags": {}
    },
    {
      "index": 2,
      "codec_name": "aac",
      "codec_type": "audio",
      "channels": 2,
      "channel_layout": "stereo",
      "sample_rate": "48000",
      "disposition": { "default": 1, "attached_pic": 0 },
      "tags": { "language": "jpn" }
    },
    {
      "index": 3,
      "codec_name": "ass",
      "codec_type": "subtitle",
      "disposition": { "default": 0 },
      "tags": { "language": "eng" }
    }
  ],
  "format": {
    "filename": "/media/test/Show.S01E01.mkv",
    "nb_streams": 4,
    "format_name": "matroska,webm",
    "format_long_name": "Matroska / WebM",
    "duration": "1437.123000",
    "size": "1234567890",
    "bit_rate": "6873456",
    "tags": { "title": "Episode 1" }
  }
}`

// SDR file with interlaced video, bitmap subtitles, and multiple audio.
const sampleInterlaced = `{
  "streams": [
    {
      "index": 0,
      "codec_name": "mpeg2video",
      "codec_type": "video",
      "profile": "Main",
      "pix_fmt": "yuv420p",
      "width": 720,
      "height": 480,
      "bit_rate": "3500000",
      "field_order": "tt",
      "color_transfer": "bt709",
      "color_primaries": "bt709",
      "color_space": "bt709",
      "avg_frame_rate": "30000/1001",
      "disposition": { "default": 1, "attached_pic": 0 },
      "tags": {}
    },
    {
      "index": 1,
      "codec_name": "ac3",
      "codec_type": "audio",
      "channels": 6,
      "channel_layout": "5.1(side)",
      "sample_rate": "48000",
      "disposition": { "default": 1, "attached_pic": 0 },
      "tags": { "language": "eng" }
    },
    {
      "index": 2,
      "codec_name": "aac",
      "codec_type": "audio",
      "channels": 2,
      "channel_layout": "stereo",
      "sample_rate": "44100",
      "disposition": { "default": 0, "attached_pic": 0 },
      "tags": { "language": "jpn" }
    },
    {
      "index": 3,
      "codec_name": "hdmv_pgs_subtitle",
      "codec_type": "subtitle",
      "disposition": { "default": 0 },
      "tags": { "language": "eng" }
    },
    {
      "index": 4,
      "codec_name": "dvd_subtitle",
      "codec_type": "subtitle",
      "disposition": { "default": 0 },
      "tags": { "language": "jpn" }
    }
  ],
  "format": {
    "filename": "/media/test/dvd_rip.mkv",
    "nb_streams": 5,
    "format_name": "matroska,webm",
    "format_long_name": "Matroska / WebM",
    "duration": "5400.000000",
    "size": "4000000000",
    "bit_rate": "5925925",
    "tags": {}
  }
}`

// Minimal file: just video, no audio, no subs.
const sampleMinimal = `{
  "streams": [
    {
      "index": 0,
      "codec_name": "h264",
      "codec_type": "video",
      "profile": "High",
      "pix_fmt": "yuv420p",
      "width": 1280,
      "height": 720,
      "field_order": "progressive",
      "disposition": { "default": 1, "attached_pic": 0 },
      "tags": {}
    }
  ],
  "format": {
    "filename": "minimal.mp4",
    "nb_streams": 1,
    "format_name": "mov,mp4,m4a,3gp,3g2,mj2",
    "duration": "10.000",
    "size": "500000",
    "bit_rate": "400000",
    "tags": {}
  }
}`

func TestParseJSON_HDRFile(t *testing.T) {
	pr, err := ParseJSON([]byte(sampleHDR))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	// Format
	if pr.Format.Filename != "/media/test/Show.S01E01.mkv" {
		t.Errorf("filename: got %q", pr.Format.Filename)
	}
	if pr.Format.NbStreams != 4 {
		t.Errorf("nb_streams: got %d, want 4", pr.Format.NbStreams)
	}
	if pr.Format.Duration != 1437.123 {
		t.Errorf("duration: got %f, want 1437.123", pr.Format.Duration)
	}
	if pr.Format.Size != 1234567890 {
		t.Errorf("size: got %d", pr.Format.Size)
	}
	if pr.Format.BitRate != 6873456 {
		t.Errorf("format bitrate: got %d", pr.Format.BitRate)
	}
	if pr.Format.Tags["title"] != "Episode 1" {
		t.Errorf("tags: got %v", pr.Format.Tags)
	}

	// Primary video should skip the mjpeg cover art (index 0)
	if pr.PrimaryVideo == nil {
		t.Fatal("PrimaryVideo is nil")
	}
	if pr.PrimaryVideo.Index != 1 {
		t.Errorf("primary video index: got %d, want 1", pr.PrimaryVideo.Index)
	}
	if pr.PrimaryVideo.Codec != "hevc" {
		t.Errorf("codec: got %q", pr.PrimaryVideo.Codec)
	}
	if pr.PrimaryVideo.Profile != "Main 10" {
		t.Errorf("profile: got %q", pr.PrimaryVideo.Profile)
	}
	if pr.PrimaryVideo.Width != 1920 || pr.PrimaryVideo.Height != 1080 {
		t.Errorf("resolution: got %dx%d", pr.PrimaryVideo.Width, pr.PrimaryVideo.Height)
	}
	if pr.PrimaryVideo.BitRate != 5000000 {
		t.Errorf("video bitrate: got %d", pr.PrimaryVideo.BitRate)
	}
	if pr.PrimaryVideo.IsAttachedPic {
		t.Error("primary video should not be attached_pic")
	}

	// Audio
	if len(pr.AudioStreams) != 1 {
		t.Fatalf("audio streams: got %d, want 1", len(pr.AudioStreams))
	}
	a := pr.AudioStreams[0]
	if a.Codec != "aac" || a.Channels != 2 || a.SampleRate != 48000 {
		t.Errorf("audio: codec=%q ch=%d sr=%d", a.Codec, a.Channels, a.SampleRate)
	}
	if a.Language != "jpn" {
		t.Errorf("audio language: got %q", a.Language)
	}
	if !a.IsDefault {
		t.Error("audio should be default")
	}

	// Subtitles
	if len(pr.SubtitleStreams) != 1 {
		t.Fatalf("subtitle streams: got %d, want 1", len(pr.SubtitleStreams))
	}
	if pr.SubtitleStreams[0].Codec != "ass" {
		t.Errorf("sub codec: got %q", pr.SubtitleStreams[0].Codec)
	}
	if pr.SubtitleStreams[0].Language != "eng" {
		t.Errorf("sub language: got %q", pr.SubtitleStreams[0].Language)
	}
	if pr.SubtitleStreams[0].IsBitmap {
		t.Error("ASS should not be bitmap")
	}
	if pr.HasBitmapSubs {
		t.Error("should not have bitmap subs")
	}
}

func TestParseJSON_InterlacedFile(t *testing.T) {
	pr, err := ParseJSON([]byte(sampleInterlaced))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	if pr.PrimaryVideo == nil {
		t.Fatal("PrimaryVideo is nil")
	}
	if pr.PrimaryVideo.Codec != "mpeg2video" {
		t.Errorf("codec: got %q", pr.PrimaryVideo.Codec)
	}
	if pr.PrimaryVideo.FieldOrder != "tt" {
		t.Errorf("field_order: got %q", pr.PrimaryVideo.FieldOrder)
	}

	// Multiple audio streams.
	if len(pr.AudioStreams) != 2 {
		t.Fatalf("audio streams: got %d, want 2", len(pr.AudioStreams))
	}
	if pr.AudioStreams[0].Channels != 6 {
		t.Errorf("first audio channels: got %d", pr.AudioStreams[0].Channels)
	}
	if pr.AudioStreams[1].SampleRate != 44100 {
		t.Errorf("second audio sample_rate: got %d", pr.AudioStreams[1].SampleRate)
	}

	// Bitmap subtitles.
	if len(pr.SubtitleStreams) != 2 {
		t.Fatalf("subtitle streams: got %d, want 2", len(pr.SubtitleStreams))
	}
	if !pr.SubtitleStreams[0].IsBitmap {
		t.Error("hdmv_pgs should be bitmap")
	}
	if !pr.SubtitleStreams[1].IsBitmap {
		t.Error("dvd_subtitle should be bitmap")
	}
	if !pr.HasBitmapSubs {
		t.Error("should have bitmap subs")
	}
}

func TestParseJSON_MinimalFile(t *testing.T) {
	pr, err := ParseJSON([]byte(sampleMinimal))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	if pr.PrimaryVideo == nil {
		t.Fatal("PrimaryVideo is nil")
	}
	if pr.PrimaryVideo.Codec != "h264" {
		t.Errorf("codec: got %q", pr.PrimaryVideo.Codec)
	}
	if len(pr.AudioStreams) != 0 {
		t.Errorf("audio streams: got %d, want 0", len(pr.AudioStreams))
	}
	if len(pr.SubtitleStreams) != 0 {
		t.Errorf("subtitle streams: got %d, want 0", len(pr.SubtitleStreams))
	}
}

func TestVideoBitRate(t *testing.T) {
	// Stream bitrate available → use it.
	pr, _ := ParseJSON([]byte(sampleHDR))
	if got := pr.VideoBitRate(); got != 5000000 {
		t.Errorf("with stream bitrate: got %d, want 5000000", got)
	}

	// Stream bitrate missing → fall back to format.
	pr, _ = ParseJSON([]byte(sampleMinimal))
	if got := pr.VideoBitRate(); got != 400000 {
		t.Errorf("fallback to format: got %d, want 400000", got)
	}
}

func TestResolution(t *testing.T) {
	pr, _ := ParseJSON([]byte(sampleHDR))
	if got := pr.Resolution(); got != "1920x1080" {
		t.Errorf("got %q, want 1920x1080", got)
	}

	pr, _ = ParseJSON([]byte(sampleMinimal))
	if got := pr.Resolution(); got != "1280x720" {
		t.Errorf("got %q, want 1280x720", got)
	}

	// No video → unknown.
	empty := &ProbeResult{}
	if got := empty.Resolution(); got != "unknown" {
		t.Errorf("got %q, want unknown", got)
	}
}

func TestHDRType(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		{"HDR smpte2084", sampleHDR, "hdr10"},
		{"SDR mpeg2", sampleInterlaced, "sdr"},
		{"SDR h264", sampleMinimal, "sdr"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr, _ := ParseJSON([]byte(tc.json))
			if got := pr.HDRType(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}

	// arib-std-b67 (HLG) also counts as HDR.
	t.Run("HLG via arib-std-b67", func(t *testing.T) {
		pr := &ProbeResult{PrimaryVideo: &VideoStream{ColorTransfer: "arib-std-b67"}}
		if got := pr.HDRType(); got != "hdr10" {
			t.Errorf("got %q, want hdr10", got)
		}
	})

	// bt2020 primaries without smpte2084 transfer still triggers HDR.
	t.Run("bt2020 primaries only", func(t *testing.T) {
		pr := &ProbeResult{PrimaryVideo: &VideoStream{ColorPrimaries: "bt2020"}}
		if got := pr.HDRType(); got != "hdr10" {
			t.Errorf("got %q, want hdr10", got)
		}
	})

	// No primary video → SDR.
	t.Run("no video", func(t *testing.T) {
		pr := &ProbeResult{}
		if got := pr.HDRType(); got != "sdr" {
			t.Errorf("got %q, want sdr", got)
		}
	})
}

func TestIsInterlaced(t *testing.T) {
	cases := []struct {
		name       string
		fieldOrder string
		want       bool
	}{
		{"progressive", "progressive", false},
		{"top-top", "tt", true},
		{"bottom-bottom", "bb", true},
		{"top-bottom", "tb", true},
		{"bottom-top", "bt", true},
		{"unknown/empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := &ProbeResult{PrimaryVideo: &VideoStream{FieldOrder: tc.fieldOrder}}
			if got := pr.IsInterlaced(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("no video", func(t *testing.T) {
		pr := &ProbeResult{}
		if pr.IsInterlaced() {
			t.Error("should be false with no video")
		}
	})

	// From full JSON sample.
	t.Run("interlaced sample", func(t *testing.T) {
		pr, _ := ParseJSON([]byte(sampleInterlaced))
		if !pr.IsInterlaced() {
			t.Error("interlaced sample should be interlaced")
		}
	})
}

func TestIsEdgeSafeHEVC(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		pixFmt  string
		want    bool
	}{
		{"main + yuv420p", "Main", "yuv420p", true},
		{"main 10 + yuv420p10le", "Main 10", "yuv420p10le", true},
		{"main10 + yuv420p10le", "main10", "yuv420p10le", true},
		{"high + yuv420p", "High", "yuv420p", false},
		{"main + yuv444p", "Main", "yuv444p", false},
		{"rext + yuv420p10le", "Rext", "yuv420p10le", false},
		{"empty profile", "", "yuv420p", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := &ProbeResult{PrimaryVideo: &VideoStream{Profile: tc.profile, PixFmt: tc.pixFmt}}
			if got := pr.IsEdgeSafeHEVC(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	// From full JSON sample (HDR file: Main 10 + yuv420p10le → safe).
	t.Run("HDR sample", func(t *testing.T) {
		pr, _ := ParseJSON([]byte(sampleHDR))
		if !pr.IsEdgeSafeHEVC() {
			t.Error("HDR sample should be edge-safe HEVC")
		}
	})

	t.Run("no video", func(t *testing.T) {
		pr := &ProbeResult{}
		if pr.IsEdgeSafeHEVC() {
			t.Error("should be false with no video")
		}
	})
}

func TestBitmapSubCodecs(t *testing.T) {
	codecs := []string{"hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "xsub"}
	for _, c := range codecs {
		t.Run(c, func(t *testing.T) {
			if !bitmapSubCodecs[c] {
				t.Errorf("%q should be bitmap", c)
			}
		})
	}

	nonBitmap := []string{"ass", "srt", "subrip", "mov_text", "webvtt"}
	for _, c := range nonBitmap {
		t.Run(c+"_not_bitmap", func(t *testing.T) {
			if bitmapSubCodecs[c] {
				t.Errorf("%q should NOT be bitmap", c)
			}
		})
	}
}

func TestParseJSON_InvalidJSON(t *testing.T) {
	_, err := ParseJSON([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseJSON_EmptyStreams(t *testing.T) {
	pr, err := ParseJSON([]byte(`{"streams":[],"format":{"filename":"empty.mkv","nb_streams":0}}`))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if pr.PrimaryVideo != nil {
		t.Error("expected nil PrimaryVideo")
	}
	if len(pr.AudioStreams) != 0 {
		t.Errorf("audio: got %d", len(pr.AudioStreams))
	}
}

func TestAttachedPicSkipped(t *testing.T) {
	// A file where the ONLY video stream is an attached pic.
	j := `{
		"streams": [
			{
				"index": 0,
				"codec_name": "mjpeg",
				"codec_type": "video",
				"width": 300, "height": 300,
				"disposition": { "attached_pic": 1 }
			},
			{
				"index": 1,
				"codec_name": "aac",
				"codec_type": "audio",
				"channels": 2,
				"sample_rate": "44100",
				"disposition": { "default": 1 }
			}
		],
		"format": { "filename": "audio_only.m4a", "nb_streams": 2 }
	}`
	pr, err := ParseJSON([]byte(j))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}
	if pr.PrimaryVideo != nil {
		t.Error("PrimaryVideo should be nil when only stream is attached_pic")
	}
}

func TestStreamBitRate_TagBPSFallback(t *testing.T) {
	// MKV-style: audio stream has no bit_rate field, but tags.BPS is present.
	j := `{
		"streams": [
			{
				"index": 0,
				"codec_name": "hevc",
				"codec_type": "video",
				"width": 1920, "height": 1080,
				"disposition": { "default": 1, "attached_pic": 0 },
				"tags": { "BPS": "5000000" }
			},
			{
				"index": 1,
				"codec_name": "flac",
				"codec_type": "audio",
				"channels": 2,
				"sample_rate": "48000",
				"disposition": { "default": 1 },
				"tags": { "language": "jpn", "BPS": "930000" }
			},
			{
				"index": 2,
				"codec_name": "aac",
				"codec_type": "audio",
				"channels": 2,
				"sample_rate": "48000",
				"bit_rate": "256000",
				"disposition": { "default": 0 },
				"tags": { "language": "eng", "BPS-eng": "192000" }
			}
		],
		"format": {
			"filename": "test.mkv",
			"nb_streams": 3,
			"format_name": "matroska,webm",
			"duration": "1400.000",
			"size": "1000000000",
			"bit_rate": "5714285",
			"tags": {}
		}
	}`

	pr, err := ParseJSON([]byte(j))
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	// Video: no bit_rate field, should fall back to tags.BPS.
	if pr.PrimaryVideo.BitRate != 5000000 {
		t.Errorf("video BitRate: got %d, want 5000000 (from tags.BPS)", pr.PrimaryVideo.BitRate)
	}

	if len(pr.AudioStreams) != 2 {
		t.Fatalf("audio streams: got %d, want 2", len(pr.AudioStreams))
	}

	// Audio[0]: flac with no bit_rate, should fall back to tags.BPS.
	if pr.AudioStreams[0].BitRate != 930000 {
		t.Errorf("audio[0] BitRate: got %d, want 930000 (from tags.BPS)", pr.AudioStreams[0].BitRate)
	}

	// Audio[1]: aac with bit_rate=256000; top-level value takes precedence over BPS-eng tag.
	if pr.AudioStreams[1].BitRate != 256000 {
		t.Errorf("audio[1] BitRate: got %d, want 256000 (from bit_rate field)", pr.AudioStreams[1].BitRate)
	}
}

func TestAudioBitRate(t *testing.T) {
	pr := &ProbeResult{
		AudioStreams: []AudioStream{{BitRate: 192000}},
	}
	if got := pr.AudioBitRate(); got != 192000 {
		t.Errorf("got %d, want 192000", got)
	}

	empty := &ProbeResult{}
	if got := empty.AudioBitRate(); got != 0 {
		t.Errorf("empty: got %d, want 0", got)
	}
}

// Verbose output for manual inspection of a realistic probe.
func TestDebugSampleProbe(t *testing.T) {
	pr, _ := ParseJSON([]byte(sampleHDR))
	t.Logf("Format: %s (%s), %d streams, %.1fs, %d bytes",
		pr.Format.FormatName, pr.Format.Filename,
		pr.Format.NbStreams, pr.Format.Duration, pr.Format.Size)
	t.Logf("Video: %s %s, %s, %d bps, field=%s",
		pr.PrimaryVideo.Codec, pr.PrimaryVideo.Profile,
		pr.Resolution(), pr.VideoBitRate(), pr.PrimaryVideo.FieldOrder)
	t.Logf("HDR: %s, Interlaced: %v, EdgeSafe: %v",
		pr.HDRType(), pr.IsInterlaced(), pr.IsEdgeSafeHEVC())
	t.Logf("Color: transfer=%s primaries=%s space=%s",
		pr.PrimaryVideo.ColorTransfer, pr.PrimaryVideo.ColorPrimaries, pr.PrimaryVideo.ColorSpace)
	for i, a := range pr.AudioStreams {
		t.Logf("Audio[%d]: %s, %dch, %dHz, lang=%s, default=%v",
			i, a.Codec, a.Channels, a.SampleRate, a.Language, a.IsDefault)
	}
	for i, s := range pr.SubtitleStreams {
		t.Logf("Sub[%d]: %s, lang=%s, bitmap=%v", i, s.Codec, s.Language, s.IsBitmap)
	}
}

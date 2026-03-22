// HDR detection from color transfer, primaries, and space metadata.
// Also provides formatting for HDR10 static metadata (mastering display
// and content light level) used by the planner and ffmpeg builder.
package probe

import (
	"fmt"
	"strings"
)

// HDRType returns "hdr10" if the primary video stream has HDR color
// metadata, otherwise "sdr". Detection mirrors the legacy detect_hdr_type
// function: smpte2084/arib-std-b67 transfer or bt2020 primaries.
func (p *ProbeResult) HDRType() string {
	if p.PrimaryVideo == nil {
		return "sdr"
	}

	switch strings.ToLower(strings.TrimSpace(p.PrimaryVideo.ColorTransfer)) {
	case "smpte2084", "arib-std-b67":
		return "hdr10"
	}

	if strings.EqualFold(strings.TrimSpace(p.PrimaryVideo.ColorPrimaries), "bt2020") {
		return "hdr10"
	}

	return "sdr"
}

// FFmpegMasterDisplay formats the mastering display metadata for ffmpeg's
// -master_display / x265 --master-display option:
//
//	G(gx,gy)B(bx,by)R(rx,ry)WP(wpx,wpy)L(maxL,minL)
func (m *MasteringDisplay) FFmpegMasterDisplay() string {
	return fmt.Sprintf("G(%d,%d)B(%d,%d)R(%d,%d)WP(%d,%d)L(%d,%d)",
		m.GreenX, m.GreenY, m.BlueX, m.BlueY, m.RedX, m.RedY,
		m.WhiteX, m.WhiteY, m.MaxLuminance, m.MinLuminance)
}

// FFmpegMaxCLL formats the content light level for ffmpeg's
// -max_cll / x265 --max-cll option: "MaxCLL,MaxFALL".
func (c *ContentLightLevel) FFmpegMaxCLL() string {
	return fmt.Sprintf("%d,%d", c.MaxCLL, c.MaxFALL)
}

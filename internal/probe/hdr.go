package probe

// HDRType returns "hdr10" if the primary video stream has HDR color
// metadata, otherwise "sdr". Detection mirrors the legacy detect_hdr_type
// function: smpte2084/arib-std-b67 transfer or bt2020 primaries.
func (p *ProbeResult) HDRType() string {
	if p.PrimaryVideo == nil {
		return "sdr"
	}

	switch p.PrimaryVideo.ColorTransfer {
	case "smpte2084", "arib-std-b67":
		return "hdr10"
	}

	if p.PrimaryVideo.ColorPrimaries == "bt2020" {
		return "hdr10"
	}

	return "sdr"
}

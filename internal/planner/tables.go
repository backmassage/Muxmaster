// tables.go centralizes lookup tables for quality curves, ratio estimation,
// and bias adjustments. Keeping all tuning data in one place makes curves
// easy to review, verify monotonicity, and adjust.
package planner

// --- Tier-based threshold lookup ---

// tier maps an upper-bound threshold to an adjustment value.
type tier struct {
	limit int
	adj   int
}

// tierLookup returns adj for the first tier where val <= limit,
// or above if val exceeds all tiers. Tiers must be ascending by limit.
func tierLookup(tiers []tier, val, above int) int {
	for _, t := range tiers {
		if val <= t.limit {
			return t.adj
		}
	}
	return above
}

// =====================================================================
// QP/CRF → permille ratio tables (indexed slices)
// =====================================================================
//
// Output-to-input permille ratios indexed by quality value minus the
// base offset. Clamped: values below the base return entry[0], values
// above return the last entry. Both tables are strictly decreasing.

const vaapiRatioBase = 14 // QP 14 → index 0

// vaapiRatios: QP 14 (930‰) through QP 36+ (180‰).
var vaapiRatios = [...]int{
	930, // QP 14
	900, // QP 15
	860, // QP 16
	820, // QP 17
	770, // QP 18
	730, // QP 19
	680, // QP 20
	640, // QP 21
	590, // QP 22
	550, // QP 23
	510, // QP 24
	470, // QP 25
	430, // QP 26
	395, // QP 27
	360, // QP 28
	330, // QP 29
	300, // QP 30
	275, // QP 31
	250, // QP 32
	230, // QP 33
	210, // QP 34
	195, // QP 35
	180, // QP 36+
}

const cpuRatioBase = 16 // CRF 16 → index 0

// cpuRatios: CRF 16 (900‰) through CRF 30+ (200‰).
var cpuRatios = [...]int{
	900, // CRF 16
	820, // CRF 17
	740, // CRF 18
	660, // CRF 19
	590, // CRF 20
	520, // CRF 21
	460, // CRF 22
	410, // CRF 23
	360, // CRF 24
	320, // CRF 25
	290, // CRF 26
	260, // CRF 27
	235, // CRF 28
	215, // CRF 29
	200, // CRF 30+
}

// =====================================================================
// Codec bias maps
// =====================================================================

// codecBiases maps source codec (lowercase) to permille ratio bias for
// the estimation model. Higher = output closer to input size.
var codecBiases = map[string]int{
	"hevc": 180, "h265": 180,
	"vp9": 150, "av1": 150,
	"h264": 100, "avc": 100, "avc1": 100,
	"mpeg2video": -60, "mpeg4": -60, "wmv3": -60, "vc1": -60,
}

// optimalCodecRatio maps source codec (lowercase) to the base output/input
// percentage used by OptimalBitrate. Default (h264) is 68.
var optimalCodecRatio = map[string]int{
	"hevc": 95, "h265": 95,
	"vp9": 90, "av1": 90,
	"mpeg2video": 45, "mpeg4": 45, "wmv3": 45, "vc1": 45,
}

// =====================================================================
// SmartQuality curve tiers
// =====================================================================
//
// Resolution, bitrate, and density adjustment tables for CPU and VAAPI
// encoder modes. Each curve function applies tierLookup against one of
// these tables. Tier limits use <= semantics; where the original code
// used strict < comparisons, the limit is threshold - 1.

// CPU resolution adjustments: higher CRF for low-res, lower for high-res.
var cpuResTiers = []tier{
	{640 * 360, 4},      // <= 360p
	{854 * 480, 3},      // <= 480p
	{1280 * 720, 2},     // <= 720p
	{1920 * 1080, 1},    // <= 1080p
	{2560*1440 - 1, 0},  // < 1440p
	{3840*2160 - 1, -1}, // < 2160p
} // above: -2

// VAAPI resolution adjustments: gentler than CPU to avoid crushing
// low-resolution content where quality loss is most visible.
var vaapiResTiers = []tier{
	{640 * 360, 3},      // <= 360p
	{854 * 480, 2},      // <= 480p
	{1280 * 720, 1},     // <= 720p
	{2560*1440 - 1, 0},  // < 1440p
	{3840*2160 - 1, -1}, // < 2160p
} // above: -2

// CPU bitrate adjustments (input kbps).
var cpuBitrateTiers = []tier{
	{1199, 2},   // < 1200
	{2499, 1},   // < 2500
	{18000, 0},  // <= 18000
	{35000, -1}, // <= 35000
} // above: -2

// VAAPI bitrate adjustments (input kbps).
var vaapiBitrateTiers = []tier{
	{1199, 2},   // < 1200
	{2499, 1},   // < 2500
	{16000, 0},  // <= 16000
	{30000, -1}, // <= 30000
} // above: -2

// CPU density adjustments (kbps per megapixel).
var cpuDensityTiers = []tier{
	{DensityUltraLow - 1, 3}, // < 1000
	{DensityLow - 1, 2},      // < 1500
	{DensityBelowAvg - 1, 1}, // < 2500
	{DensityHigh, 0},         // <= 8000
} // above: -1

// VAAPI density adjustments (kbps per megapixel).
var vaapiDensityTiers = []tier{
	{DensityUltraLow - 1, 4}, // < 1000
	{DensityLow - 1, 2},      // < 1500
	{DensityBelowAvg - 1, 1}, // < 2500
	{DensityHigh, 0},         // <= 8000
} // above: -2

// =====================================================================
// Estimation bias tiers
// =====================================================================

// resBiasTiers: estimation ratio bias by resolution (pixels).
var resBiasTiers = []tier{
	{854 * 480, 80},    // <= 480p
	{1280 * 720, 40},   // <= 720p
	{3840*2160 - 1, 0}, // < 2160p
} // above: -40

// bitrateBiasTiers: estimation ratio bias by source kbps.
var bitrateBiasTiers = []tier{
	{1499, 120},  // < 1500
	{2999, 70},   // < 3000
	{15000, 0},   // <= 15000
	{30000, -20}, // <= 30000
} // above: -50

// estDensityTiers: estimation ratio bias by bitrate density.
var estDensityTiers = []tier{
	{DensityUltraLow - 1, 250}, // < 1000
	{DensityLow - 1, 150},      // < 1500
	{DensityBelowAvg - 1, 80},  // < 2500
	{DensityMedium - 1, 20},    // < 3500
	{DensityHigh, 0},           // <= 8000
	{DensityVeryHigh, -10},     // <= 10000
} // above: -20

// optDensityTiers: density adjustment for OptimalBitrate base ratio.
var optDensityTiers = []tier{
	{DensityUltraLow - 1, 30}, // < 1000
	{DensityLow - 1, 25},      // < 1500
	{DensityBelowAvg - 1, 12}, // < 2500
	{DensityHigh, 0},          // <= 8000
	{DensityVeryHigh, -5},     // <= 10000
} // above: -8

package exif

import (
	"fmt"
	"math"
	"strings"
)

// absFloat guards the small set of math helpers used by the parser.
func absFloat(v float64) float64 {
	return math.Abs(v)
}

// dmsToDegrees converts an EXIF GPS latitude/longitude (three rationals:
// degrees, minutes, seconds) into a non-signed decimal degree value.
func dmsToDegrees(rs []rational) (float64, bool) {
	if len(rs) < 3 || rs[0].den == 0 || rs[1].den == 0 || rs[2].den == 0 {
		return 0, false
	}
	deg := float64(rs[0].num) / float64(rs[0].den)
	min := float64(rs[1].num) / float64(rs[1].den)
	sec := float64(rs[2].num) / float64(rs[2].den)
	return deg + min/60 + sec/3600, true
}

// signedHemisphere applies the GPS reference letter to a decimal degree:
// rational DMS fields are stored as positive values, the reference letter
// carries the direction. A ref equal to the "negative" letter flips the sign.
func signedHemisphere(deg float64, ref string, bad byte) float64 {
	abs := math.Abs(deg)
	if strings.HasPrefix(ref, string(rune(bad))) {
		return -abs
	}
	return abs
}

// FormatDMS renders signed decimal degrees as e.g. "55°45′8.3″N" (lat=true)
// or "37°36′56.0″E" (lat=false). The hemisphere letter matches the sign.
func FormatDMS(deg float64, lat bool) string {
	hemisphere := "N"
	switch {
	case deg < 0 && lat:
		hemisphere = "S"
	case deg >= 0 && lat:
		hemisphere = "N"
	case deg < 0:
		hemisphere = "W"
	default:
		hemisphere = "E"
	}

	total := math.Abs(deg) * 3600
	d := int(total / 3600)
	rem := total - float64(d)*3600
	m := int(rem / 60)
	s := rem - float64(m)*60
	if math.Round(s*10) >= 600 { // rounding would render 60.0″
		s = 0
		if m++; m >= 60 {
			m = 0
			if d++; d >= 360 {
				d = 359
			}
		}
	}
	return fmt.Sprintf("%d°%d′%.1f″%s", d, m, s, hemisphere)
}

// FormatCoordPair renders both coordinates as one DMS string.
func FormatCoordPair(lat, lon float64) string {
	return FormatDMS(lat, true) + ", " + FormatDMS(lon, false)
}

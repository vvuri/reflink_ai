package exif

import (
	"math"
	"strconv"
)

// OrientationText maps EXIF orientation codes to short display labels
// (the UI language is English). Code 1 and 0 mean "as stored".
func OrientationText(code int) string {
	switch code {
	case 1:
		return "Normal"
	case 2:
		return "Mirrored horizontally"
	case 3:
		return "Rotated 180°"
	case 4:
		return "Mirrored vertically"
	case 5:
		return "Mirrored, rotated 90°"
	case 6:
		return "Rotated 90° CW"
	case 7:
		return "Mirrored, rotated 270°"
	case 8:
		return "Rotated 90° CCW"
	}
	return ""
}

// FlashText describes flash usage from the Flash tag value (bit 0 = fired).
func FlashText(code int) string {
	if code&0x01 == 1 {
		return "Fired"
	}
	return "Did not fire"
}

// MeteringText maps the MeteringMode tag to a readable label.
func MeteringText(code int) string {
	switch code {
	case 1:
		return "Average"
	case 2:
		return "Center-weighted"
	case 3:
		return "Spot"
	case 4:
		return "Multi-spot"
	case 5:
		return "Pattern"
	case 6:
		return "Partial"
	case 255:
		return "Other"
	}
	return ""
}

// WhiteBalanceText maps the WhiteBalance tag to a readable label.
func WhiteBalanceText(code int) string {
	switch code {
	case 0:
		return "Auto"
	case 1:
		return "Manual"
	}
	return ""
}

// FormatExposure renders an exposure time as photographers write it:
// "1/120 s" for sub-second values, "5 s" for long ones.
func FormatExposure(sec float64) string {
	switch {
	case sec <= 0:
		return ""
	case sec > 0 && sec < 1:
		want := 1 / sec
		if n := math.Round(want); n > 0 && math.Abs(want-n)/n <= 0.05 {
			return "1/" + strconv.Itoa(int(n)) + " s"
		}
		return strconv.FormatFloat(sec, 'f', -1, 64) + " s"
	case math.Abs(sec-math.Round(sec)) < 1e-9:
		return strconv.Itoa(int(sec)) + " s"
	}
	return strconv.FormatFloat(math.Round(sec*1000)/1000, 'f', -1, 64) + " s"
}

// FormatAperture renders an f-number like "f/1.78".
func FormatAperture(n float64) string {
	if n <= 0 {
		return ""
	}
	return "f/" + formatNumber(n)
}

// FormatFocalLength renders focal length like "6.86 mm".
func FormatFocalLength(mm float64) string {
	if mm <= 0 {
		return ""
	}
	return formatNumber(mm) + " mm"
}

// formatNumber prints a float without trailing zeros (1.78, 6, 6.86).
func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

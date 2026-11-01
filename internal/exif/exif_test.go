package exif_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"reflink/internal/exif"
	"reflink/internal/testtif"
)

// TestParseFull reads the shared "smartphone photo" fixture and checks
// every field the service reports.
func TestParseFull(t *testing.T) {
	d, err := exif.Parse(testtif.Phone())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !d.Present {
		t.Fatal("expected Present=true")
	}

	if d.Make != "Apple" || d.Model != "iPhone 15 Pro" || d.Software != "iOS 26.0" {
		t.Fatalf("identity fields: %+v", d)
	}
	if d.DateTime != "2026:07:14 18:32:01" || d.DateTimeOriginal != "2026:07:14 18:32:01" {
		t.Fatalf("dates: %q / %q", d.DateTime, d.DateTimeOriginal)
	}
	if d.ISO != 125 || d.Orientation != 6 || d.FocalLength35 != 26 {
		t.Fatalf("numerics: ISO=%d orientation=%d f35=%d", d.ISO, d.Orientation, d.FocalLength35)
	}
	if d.Flash != 1 || d.MeteringMode != 5 || d.WhiteBalance != 0 {
		t.Fatalf("modes: flash=%d metering=%d wb=%d", d.Flash, d.MeteringMode, d.WhiteBalance)
	}

	// Rationals: 1/120 s, f/89/50, 6.86 mm.
	if math.Abs(d.ExposureTime-1.0/120.0) > 1e-12 {
		t.Fatalf("exposure: %v", d.ExposureTime)
	}
	if math.Abs(d.FNumber-1.78) > 1e-9 {
		t.Fatalf("fnumber: %v", d.FNumber)
	}
	if math.Abs(d.FocalLength-6.86) > 1e-9 {
		t.Fatalf("focal: %v", d.FocalLength)
	}

	// GPS: 55°45′8.3″N, 37°36′56.0″E, 151.5 m above sea level.
	if !d.GPS.Present {
		t.Fatal("expected GPS.Present")
	}
	if math.Abs(d.GPS.Latitude-55.75230556) > 1e-6 {
		t.Fatalf("latitude: %v", d.GPS.Latitude)
	}
	if math.Abs(d.GPS.Longitude-37.61555556) > 1e-6 {
		t.Fatalf("longitude: %v", d.GPS.Longitude)
	}
	if !d.GPS.HasAltitude || d.GPS.AltitudeBelow {
		t.Fatalf("altitude flags: %+v", d.GPS)
	}
	if math.Abs(d.GPS.Altitude-151.5) > 1e-9 {
		t.Fatalf("altitude: %v", d.GPS.Altitude)
	}
}

// TestSouthWestAndBelowSea flips hemispheres and puts the altitude below
// sea level to check sign handling.
func TestSouthWestAndBelowSea(t *testing.T) {
	tiff := (&testtif.TIFF{
		Gps: []testtif.Entry{
			testtif.String(0x0001, "S"),
			testtif.Rats(0x0002, [][2]uint32{{33, 1}, {50, 1}, {0, 1}}),
			testtif.String(0x0003, "W"),
			testtif.Rats(0x0004, [][2]uint32{{37, 1}, {36, 1}, {5600, 100}}),
			testtif.Byte(0x0005, 1),
			testtif.Rats(0x0006, [][2]uint32{{15, 1}}),
		},
	}).Build()

	d, err := exif.Parse(tiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !d.GPS.Present {
		t.Fatal("expected GPS.Present")
	}
	if want := -(33.0 + 50.0/60.0); math.Abs(d.GPS.Latitude-want) > 1e-9 {
		t.Fatalf("latitude: %v (want %v)", d.GPS.Latitude, want)
	}
	if want := -(37.0 + 36.0/60.0 + 56.0/3600.0); math.Abs(d.GPS.Longitude-want) > 1e-9 {
		t.Fatalf("longitude: %v (want %v)", d.GPS.Longitude, want)
	}
	if !d.GPS.AltitudeBelow || math.Abs(d.GPS.Altitude-15) > 1e-9 {
		t.Fatalf("altitude: %+v", d.GPS)
	}
}

// TestNoFixZeroZero verifies the "0°0′0″ everywhere" trap is not reported
// as a location.
func TestNoFixZeroZero(t *testing.T) {
	tiff := (&testtif.TIFF{
		Gps: []testtif.Entry{
			testtif.String(0x0001, "N"),
			testtif.Rats(0x0002, [][2]uint32{{0, 1}, {0, 1}, {0, 1}}),
			testtif.String(0x0003, "E"),
			testtif.Rats(0x0004, [][2]uint32{{0, 1}, {0, 1}, {0, 1}}),
		},
	}).Build()

	d, err := exif.Parse(tiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.GPS.Present {
		t.Fatal("0/0 coordinates must not count as a location")
	}
}

// TestBigEndian checks the "MM" byte order path minimally by hand.
func TestBigEndian(t *testing.T) {
	b := new(bytes.Buffer)
	b.WriteString("MM")
	_ = binary.Write(b, binary.BigEndian, uint16(42))
	_ = binary.Write(b, binary.BigEndian, uint32(8))
	heapOff := uint32(8 + 2 + 12 + 4)
	_ = binary.Write(b, binary.BigEndian, uint16(1)) // entry count
	_ = binary.Write(b, binary.BigEndian, uint16(0x010F))
	_ = binary.Write(b, binary.BigEndian, uint16(2))
	_ = binary.Write(b, binary.BigEndian, uint32(6))
	_ = binary.Write(b, binary.BigEndian, uint32(heapOff))
	_ = binary.Write(b, binary.BigEndian, uint32(0))
	b.WriteString("Canon\x00")

	d, err := exif.Parse(b.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !d.Present || d.Make != "Canon" {
		t.Fatalf("big-endian parse failed: present=%v make=%q", d.Present, d.Make)
	}
}

// TestMalformedNoPanic throws assorted damaged payloads at the parser: it
// must never panic and must report absence of data.
func TestMalformedNoPanic(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte("II"),
		[]byte("XXjunkpayload"),
		append([]byte("II\x2a\x00"), 0xFF, 0xFF, 0xFF, 0xFF, 1, 2, 3, 4),
		append([]byte("II\x2a\x00"), 0x10, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF),
	}
	for i, c := range cases {
		d, err := exif.Parse(c)
		if d == nil {
			t.Fatalf("case %d: nil data returned", i)
		}
		if err != nil && d.Present {
			t.Fatalf("case %d: error with Present=true", i)
		}
		if err == nil && d.Present {
			t.Fatalf("case %d: junk parsed as data: %+v", i, d)
		}
	}
}

// TestDisplayHelpers pins the formatting used by the UI.
func TestDisplayHelpers(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"exposure fast", exif.FormatExposure(1.0 / 120.0), "1/120 s"},
		{"exposure very fast", exif.FormatExposure(0.001), "1/1000 s"},
		{"exposure half", exif.FormatExposure(0.5), "1/2 s"},
		{"exposure long", exif.FormatExposure(2), "2 s"},
		{"exposure zero", exif.FormatExposure(0), ""},
		{"aperture", exif.FormatAperture(1.78), "f/1.78"},
		{"aperture round", exif.FormatAperture(2), "f/2"},
		{"focal", exif.FormatFocalLength(6.86), "6.86 mm"},
		{"orient 6", exif.OrientationText(6), "Rotated 90° CW"},
		{"orient unknown", exif.OrientationText(0), ""},
		{"flash off", exif.FlashText(0), "Did not fire"},
		{"flash on", exif.FlashText(1), "Fired"},
		{"metering", exif.MeteringText(5), "Pattern"},
		{"wb", exif.WhiteBalanceText(0), "Auto"},
		{"dms pair", exif.FormatCoordPair(55.75230556, 37.61555556),
			"55°45′8.3″N, 37°36′56.0″E"},
		{"dms west", exif.FormatDMS(-33.5, false), "33°30′0.0″W"},
		{"dms south", exif.FormatDMS(-12.0, true), "12°0′0.0″S"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestEmptyStringFilter checks that sensor noize in ASCII tags is dropped.
func TestEmptyStringFilter(t *testing.T) {
	tiff := (&testtif.TIFF{
		Ifd0: []testtif.Entry{
			testtif.String(0x010F, "\x01\x02Canon  \x0b"),
		},
	}).Build()
	d, err := exif.Parse(tiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if d.Make != "Canon" {
		t.Fatalf("Make: %q", d.Make)
	}
	if strings.Contains(d.Make, "\x01") {
		t.Fatal("control characters leaked into make")
	}
}

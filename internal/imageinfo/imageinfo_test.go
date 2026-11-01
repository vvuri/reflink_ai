package imageinfo_test

import (
	"errors"
	"testing"

	"reflink/internal/exif"
	"reflink/internal/imageinfo"
	"reflink/internal/testtif"
)

var tiff = testtif.Phone() // rich EXIF payload shared by the fixtures

func want(t *testing.T, res *imageinfo.Result, err error, format string, w, h int) {
	t.Helper()
	if err != nil {
		t.Fatalf("Parse(%s): %v", format, err)
	}
	if res.Format != format || res.Width != w || res.Height != h {
		t.Fatalf("Parse(%s): got %s %dx%d, want %s %dx%d",
			format, res.Format, res.Width, res.Height, format, w, h)
	}
}

func TestParsePlainContainers(t *testing.T) {
	res, err := imageinfo.Parse(testtif.JPEG(4, 3))
	want(t, res, err, imageinfo.FormatJPEG, 4, 3)
	if len(res.EXIF) != 0 {
		t.Fatalf("plain JPEG must have no EXIF, got %d bytes", len(res.EXIF))
	}

	res, err = imageinfo.Parse(testtif.PNG(7, 5))
	want(t, res, err, imageinfo.FormatPNG, 7, 5)

	res, err = imageinfo.Parse(testtif.GIF(9, 11))
	want(t, res, err, imageinfo.FormatGIF, 9, 11)

	res, err = imageinfo.Parse(testtif.BMP(37, 15))
	want(t, res, err, imageinfo.FormatBMP, 37, 15)

	// Top-down BMP stores a negative height.
	res, err = imageinfo.Parse(testtif.BMP(37, -15))
	want(t, res, err, imageinfo.FormatBMP, 37, 15)

	res, err = imageinfo.Parse(testtif.WebPVP8(37, 15))
	want(t, res, err, imageinfo.FormatWebP, 37, 15)

	res, err = imageinfo.Parse(testtif.WebPVP8L(700, 420))
	want(t, res, err, imageinfo.FormatWebP, 700, 420)
}

func TestJPEGWithEXIF(t *testing.T) {
	res, err := imageinfo.Parse(testtif.JPEGWithEXIF(tiff))
	want(t, res, err, imageinfo.FormatJPEG, 4, 3)
	if len(res.EXIF) == 0 {
		t.Fatal("expected EXIF payload")
	}
	d, err := exif.Parse(res.EXIF)
	if err != nil || !d.Present {
		t.Fatalf("payload unreadable: %+v err=%v", d, err)
	}
	if d.Model != "iPhone 15 Pro" {
		t.Fatalf("unexpected payload content: %q", d.Model)
	}
}

func TestPNGWithEXIF(t *testing.T) {
	res, err := imageinfo.Parse(testtif.PNGWithEXIF(tiff))
	want(t, res, err, imageinfo.FormatPNG, 7, 5)
	if len(res.EXIF) == 0 {
		t.Fatal("expected eXIf chunk payload")
	}
}

func TestWebPWithEXIF(t *testing.T) {
	res, err := imageinfo.Parse(testtif.WebPVP8X(3456, 2304, tiff))
	want(t, res, err, imageinfo.FormatWebP, 3456, 2304)
	if len(res.EXIF) == 0 {
		t.Fatal("expected EXIF chunk payload")
	}
}

func TestUnsupported(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("just some plain text, obviously not an image"),
		{}, // empty body
	} {
		_, err := imageinfo.Parse(data)
		if !errors.Is(err, imageinfo.ErrUnsupported) {
			t.Fatalf("expected ErrUnsupported, got %v", err)
		}
	}
}

func TestMalformedRecognizedContainer(t *testing.T) {
	// Recognized PNG signature but a garbage IHDR.
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
		0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x00, 0x00, 0x42)
	_, err := imageinfo.Parse(png)
	if err == nil || errors.Is(err, imageinfo.ErrUnsupported) {
		t.Fatalf("expected a malformed error, got %v", err)
	}

	// JPEG signature with corrupted markers.
	jpg := []byte{0xFF, 0xD8, 0xFF, 0x00, 0x00, 0x01, 0x02, 0x03}
	_, err = imageinfo.Parse(jpg)
	if err == nil || errors.Is(err, imageinfo.ErrUnsupported) {
		t.Fatalf("expected a malformed error, got %v", err)
	}

	// BMP without a plausible DIB header.
	bmp := []byte{'B', 'M', 0x01, 0x02, 0x03, 0x04, 0, 0, 0, 0, 0, 0,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	_, err = imageinfo.Parse(bmp)
	if err == nil || errors.Is(err, imageinfo.ErrUnsupported) {
		t.Fatalf("expected a malformed error, got %v", err)
	}
}

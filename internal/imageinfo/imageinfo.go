// Package imageinfo detects the container format of an image, extracts its
// pixel dimensions and pulls out the raw EXIF payload when the container
// carries one (JPEG APP1, PNG eXIf, WebP EXIF chunk).
//
// Everything in this package is built on the standard library only.
package imageinfo

import "fmt"

// Canonical format names reported by Parse.
const (
	FormatJPEG = "JPEG"
	FormatPNG  = "PNG"
	FormatWebP = "WebP"
	FormatGIF  = "GIF"
	FormatBMP  = "BMP"
)

// ErrUnsupported reports that the byte stream is not a recognized image
// container (as opposed to a recognized but damaged one).
var ErrUnsupported = fmt.Errorf("unsupported or unrecognized image format")

// Result is what Parse understood about an image byte stream.
type Result struct {
	Format string // one of FormatJPEG..FormatBMP
	Width  int
	Height int
	EXIF   []byte // raw TIFF payload; nil when the container has none
}

// Parse inspects the full uploaded file and fills the result.
// The returned error is ErrUnsupported for unknown signatures, otherwise a
// malformed-container error carrying the format name and a short reason.
func Parse(data []byte) (*Result, error) {
	format := detectFormat(data)
	if format == "" {
		return nil, ErrUnsupported
	}

	var (
		res *Result
		err error
	)
	switch format {
	case FormatJPEG:
		res, err = parseJPEG(data)
	case FormatPNG:
		res, err = parsePNG(data)
	case FormatWebP:
		res, err = parseWEBP(data)
	case FormatGIF:
		res, err = parseGIF(data)
	case FormatBMP:
		res, err = parseBMP(data)
	}
	if err != nil {
		return nil, fmt.Errorf("malformed %s: %w", format, err)
	}
	return res, nil
}

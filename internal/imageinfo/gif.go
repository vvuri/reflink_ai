package imageinfo

import (
	"bytes"
	"errors"
	"image/gif"
)

var (
	errTruncated = errors.New("truncated container")
	errNoDims    = errors.New("no dimension chunk found")
)

// parseGIF reads dimensions via the standard library. GIF never carries EXIF.
func parseGIF(data []byte) (*Result, error) {
	cfg, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &Result{
		Format: FormatGIF,
		Width:  cfg.Width,
		Height: cfg.Height,
	}, nil
}

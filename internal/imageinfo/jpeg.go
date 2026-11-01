package imageinfo

import (
	"bytes"
	"encoding/binary"
	"image/jpeg"
)

// parseJPEG reads dimensions via the standard library and scans the marker
// stream for the first APP1 "Exif" segment.
func parseJPEG(data []byte) (*Result, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &Result{
		Format: FormatJPEG,
		Width:  cfg.Width,
		Height: cfg.Height,
		EXIF:   jpegEXIF(data),
	}, nil
}

// jpegEXIF walks the JPEG marker stream and returns the TIFF payload of the
// first APP1 segment that starts with "Exif\0\0", or nil when there is none.
// It never reads past the marker stream (entropy data is never touched).
func jpegEXIF(data []byte) []byte {
	pos := 2 // skip SOI
	for iter := 0; pos+4 <= len(data) && iter < 4096; iter++ {
		if data[pos] != 0xFF {
			return nil
		}
		// Collapse redundant padding FFs before a marker.
		adv := 1
		for pos+adv < len(data) && data[pos+adv] == 0xFF {
			adv++
		}
		marker := data[pos+adv]
		seg := pos + adv + 1

		switch {
		case marker == 0xDA: // SOS: entropy-coded data follows
			return nil
		case marker == 0x01, marker >= 0xD0 && marker <= 0xD9: // no length field
			pos = seg
		default:
			if seg+2 > len(data) {
				return nil
			}
			segLen := int(binary.BigEndian.Uint16(data[seg : seg+2]))
			if segLen < 2 {
				return nil
			}
			payload := seg + 2
			end := payload + segLen - 2
			if end > len(data) {
				return nil
			}
			if marker == 0xE1 && end-payload >= 6 &&
				bytes.Equal(data[payload:payload+6], []byte("Exif\x00\x00")) {
				return data[payload+6 : end]
			}
			pos = end
		}
	}
	return nil
}

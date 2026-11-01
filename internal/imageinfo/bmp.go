package imageinfo

import (
	"encoding/binary"
)

// parseBMP reads dimensions from the DIB header that follows the 14-byte
// file header. Covers BITMAPCOREHEADER (12 bytes) and every INFO/V4/V5
// variant (>= 40 bytes).
func parseBMP(data []byte) (*Result, error) {
	if len(data) < 26 {
		return nil, errTruncated
	}
	dibSize := binary.LittleEndian.Uint32(data[14:18])

	var w, h int
	switch dibSize {
	case 12: // BITMAPCOREHEADER
		if len(data) < 22 {
			return nil, errTruncated
		}
		w = int(binary.LittleEndian.Uint16(data[18:20]))
		h = int(binary.LittleEndian.Uint16(data[20:22]))
	default:
		w = int(int32(binary.LittleEndian.Uint32(data[18:22])))
		h = int(int32(binary.LittleEndian.Uint32(data[22:26])))
	}
	if h < 0 {
		h = -h // top-down bitmap
	}
	if w <= 0 || h <= 0 {
		return nil, errNoDims
	}
	return &Result{
		Format: FormatBMP,
		Width:  w,
		Height: h,
	}, nil
}

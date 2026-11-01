package imageinfo

import (
	"encoding/binary"
)

// parseWEBP handles all three payload flavors of the RIFF container:
//
//   - "VP8 "  simple lossy    — dimensions in the VP8 keyframe header
//   - "VP8L"  lossless        — dimensions packed in 14-bit fields
//   - "VP8X"  extended        — canvas size uses 24-bit fields
//
// Unknown chunks are skipped; the first EXIF chunk found (per the WebP
// container spec it normally follows VP8X) becomes the EXIF payload.
func parseWEBP(data []byte) (*Result, error) {
	if len(data) < 12 {
		return nil, errTruncated
	}
	end := len(data)
	if sz := binary.LittleEndian.Uint32(data[4:8]); sz > 0 {
		if declared := 8 + int(sz); declared < end {
			end = declared // clamp to the RIFF-declared size
		}
	}

	res := &Result{Format: FormatWebP}
	pos := 12
	first := true
	for pos+8 <= end {
		typ := data[pos : pos+4]
		n := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		start := pos + 8
		if n < 0 || start+n > end {
			break
		}
		payload := data[start : start+n]

		switch {
		case string(typ) == "VP8X" && len(payload) >= 10:
			// payload: flags(1) reserved(3) canvasWidthMinus1(3) canvasHeightMinus1(3)
			res.Width = int(uint32(payload[4])|uint32(payload[5])<<8|uint32(payload[6])<<16) + 1
			res.Height = int(uint32(payload[7])|uint32(payload[8])<<8|uint32(payload[9])<<16) + 1
		case first && string(typ) == "VP8 " && len(payload) >= 10 &&
			payload[3] == 0x9D && payload[4] == 0x01 && payload[5] == 0x2A:
			// keyframe: start code 9D 01 2A, then 14-bit width and height
			res.Width = int(binary.LittleEndian.Uint16(payload[6:8]) & 0x3FFF)
			res.Height = int(binary.LittleEndian.Uint16(payload[8:10]) & 0x3FFF)
		case first && string(typ) == "VP8L" && len(payload) >= 5 && payload[0] == 0x2F:
			// lossless: signature 0x2F, then width-1 and height-1 in 14-bit fields
			n32 := binary.LittleEndian.Uint32(payload[1:5])
			res.Width = int(n32&0x3FFF) + 1
			res.Height = int((n32>>14)&0x3FFF) + 1
		case string(typ) == "EXIF":
			res.EXIF = stripExifPrefix(payload)
		}

		first = false
		pos = start + n + n&1 // chunks are padded to even sizes
	}

	if res.Width <= 0 || res.Height <= 0 {
		return nil, errNoDims
	}
	return res, nil
}

// stripExifPrefix tolerates a JPEG-style "Exif\0\0" header inside the chunk.
func stripExifPrefix(payload []byte) []byte {
	if len(payload) >= 6 && string(payload[:6]) == "Exif\x00\x00" {
		return payload[6:]
	}
	return payload
}

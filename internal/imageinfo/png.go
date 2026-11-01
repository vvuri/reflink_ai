package imageinfo

import (
	"bytes"
	"encoding/binary"
	"image/png"
)

// parsePNG reads dimensions via the standard library and walks the chunk
// stream for the first eXIf chunk (PNG 1.5 extension).
func parsePNG(data []byte) (*Result, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &Result{
		Format: FormatPNG,
		Width:  cfg.Width,
		Height: cfg.Height,
		EXIF:   pngEXIF(data),
	}, nil
}

// pngEXIF walks the chunk stream and returns the payload of the first eXIf
// chunk, or nil. Chunk layout: 4-byte length (big endian), 4-byte type,
// payload, 4-byte CRC.
func pngEXIF(data []byte) []byte {
	pos := 8 // skip the 8-byte signature
	for pos+8 <= len(data) {
		n := binary.BigEndian.Uint32(data[pos : pos+4])
		typ := data[pos+4 : pos+8]
		start := pos + 8
		if uint64(start)+uint64(n) > uint64(len(data)) {
			return nil
		}
		if n == 0 && start+4 > len(data) {
			return nil
		}
		if string(typ) == "eXIf" {
			return data[start : start+int(n)]
		}
		if string(typ) == "IEND" {
			return nil
		}
		pos = start + int(n) + 4 // skip payload + CRC
	}
	return nil
}

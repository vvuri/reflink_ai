package imageinfo

import "bytes"

// detectFormat sniffs the leading bytes of data and returns a canonical
// format name, or "" for unknown signatures.
func detectFormat(data []byte) string {
	// JPEG: first three bytes of every reasonable JPEG stream.
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return FormatJPEG
	}
	// PNG: 8-byte signature.
	if len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return FormatPNG
	}
	// GIF: version string right after the magic.
	if len(data) >= 6 &&
		(bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))) {
		return FormatGIF
	}
	// BMP: "BM" file header.
	if len(data) >= 2 && data[0] == 'B' && data[1] == 'M' {
		return FormatBMP
	}
	// WebP: RIFF container with WEBP form type.
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return FormatWebP
	}
	return ""
}

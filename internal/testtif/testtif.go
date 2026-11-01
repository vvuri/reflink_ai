// Package testtif assembles synthetic EXIF/TIFF payloads and complete
// image fixtures (JPEG/PNG/WebP/GIF/BMP) used by the unit tests.
// It lives under internal/, so only this module can import it.
package testtif

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
)

// Entry is one ready-encoded IFD entry value.
type Entry struct {
	Tag uint16
	Typ uint16
	Val []byte

	pos int64 // heap position, filled by TIFF.Build
}

// String encodes an ASCII entry with a NUL terminator.
func String(tag uint16, s string) Entry {
	return Entry{Tag: tag, Typ: 2, Val: append([]byte(s), 0)}
}

// Short encodes a single SHORT value.
func Short(tag uint16, v uint16) Entry {
	return Entry{Tag: tag, Typ: 3, Val: le16(v)}
}

// Long encodes a single LONG value.
func Long(tag uint16, v uint32) Entry {
	return Entry{Tag: tag, Typ: 4, Val: le32(v)}
}

// Byte encodes a single BYTE value.
func Byte(tag uint16, v byte) Entry {
	return Entry{Tag: tag, Typ: 1, Val: []byte{v}}
}

// Rats encodes RATIONAL values as {num, den} pairs.
func Rats(tag uint16, rs [][2]uint32) Entry {
	var b bytes.Buffer
	for _, r := range rs {
		b.Write(le32(r[0]))
		b.Write(le32(r[1]))
	}
	return Entry{Tag: tag, Typ: 5, Val: b.Bytes()}
}

// TIFF assembles a little-endian TIFF payload in three parts.
type TIFF struct {
	Ifd0, Exif, Gps []Entry
}

// Build serializes the payload: header, IFD0 (with automatic sub-IFD
// pointers), Exif IFD, GPS IFD and a 4-byte-aligned value heap.
func (t *TIFF) Build() []byte {
	const hdr = 8

	ifd0 := append([]Entry{}, t.Ifd0...)
	exifIdx, gpsIdx := -1, -1
	if len(t.Exif) > 0 {
		ifd0 = append(ifd0, Entry{Tag: 0x8769, Typ: 4})
		exifIdx = len(ifd0) - 1
	}
	if len(t.Gps) > 0 {
		ifd0 = append(ifd0, Entry{Tag: 0x8825, Typ: 4})
		gpsIdx = len(ifd0) - 1
	}

	exifOff := uint32(hdr + ifdBytes(len(ifd0)))
	gpsOff := exifOff + uint32(ifdBytes(len(t.Exif)))
	heapOff := int64(gpsOff + uint32(ifdBytes(len(t.Gps))))

	var heap bytes.Buffer
	allocate := func(es []Entry) {
		for i := range es {
			if len(es[i].Val) <= 4 {
				continue // value is stored inline
			}
			for heap.Len()%4 != 0 {
				heap.WriteByte(0) // keep 4-byte value alignment
			}
			es[i].pos = heapOff + int64(heap.Len())
			heap.Write(es[i].Val)
		}
	}
	allocate(ifd0)
	allocate(t.Exif)
	allocate(t.Gps)

	if exifIdx >= 0 {
		ifd0[exifIdx].Val = le32(exifOff)
	}
	if gpsIdx >= 0 {
		ifd0[gpsIdx].Val = le32(gpsOff)
	}

	out := new(bytes.Buffer)
	out.WriteString("II")
	out.Write(le16(42))
	out.Write(le32(uint32(hdr)))
	write := func(es []Entry) {
		out.Write(le16(uint16(len(es))))
		for _, e := range es {
			out.Write(le16(e.Tag))
			out.Write(le16(e.Typ))
			// TIFF counts ELEMENTS of the type, not bytes: one LONG is
			// count=1 even though the payload is 4 bytes.
			out.Write(le32(uint32(elementCount(e.Typ, e.Val))))
			if len(e.Val) > 4 {
				out.Write(le32(uint32(e.pos)))
			} else {
				v := make([]byte, 4)
				copy(v, e.Val)
				out.Write(v)
			}
		}
		out.Write(le32(0)) // no IFD1
	}
	write(ifd0)
	write(t.Exif)
	write(t.Gps)
	out.Write(heap.Bytes())
	return out.Bytes()
}

func ifdBytes(n int) int { return 2 + 12*n + 4 }

// elementCount converts a value length into the TIFF count field:
// elements of the declared type, not raw bytes.
func elementCount(typ uint16, data []byte) int {
	switch typ {
	case 1, 2, 6, 7: // BYTE, ASCII, SBYTE, UNDEFINED
		return len(data)
	case 3, 8: // SHORT, USHORT
		return len(data) / 2
	case 4, 9, 11: // LONG, SLONG, FLOAT
		return len(data) / 4
	case 5, 10, 12: // RATIONAL, SRATIONAL, DOUBLE
		return len(data) / 8
	}
	return 0
}

func le16(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

func le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func leI32(v int32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	return b
}

func leu24(v uint32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16)}
}

/* ---------- whole-image fixtures ---------- */

// JPEG encodes a solid (w,h)-sized JPEG via the standard library.
func JPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{200, 100, 50, 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// JPEGWithEXIF injects an APP1 "Exif" segment into a plain JPEG.
func JPEGWithEXIF(tiff []byte) []byte {
	j := JPEG(4, 3)
	segLen := 2 + 6 + len(tiff) // length bytes + "Exif\0\0" + TIFF
	seg := []byte{0xFF, 0xE1}   // APP1 marker
	seg = append(seg, byte(segLen>>8), byte(segLen))
	seg = append(seg, []byte("Exif\x00\x00")...)
	seg = append(seg, tiff...)

	out := make([]byte, 0, len(j)+len(seg))
	out = append(out, j[:2]...) // SOI
	out = append(out, seg...)
	return append(out, j[2:]...)
}

// PNG encodes a solid (w,h)-sized PNG.
func PNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(1, 1, color.RGBA{20, 160, 120, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// PNGWithEXIF inserts an eXIf chunk right before IEND.
func PNGWithEXIF(tiff []byte) []byte {
	p := PNG(7, 5)
	i := len(p) - 12 // IEND is the last 12 bytes (len 0 + type + fixed CRC)

	chunk := make([]byte, 8)
	binary.BigEndian.PutUint32(chunk[0:4], uint32(len(tiff)))
	copy(chunk[4:8], "eXIf")
	chunk = append(chunk, tiff...)
	var crcBuf bytes.Buffer
	crcBuf.Write([]byte("eXIf"))
	crcBuf.Write(tiff)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(crcBuf.Bytes()))

	out := make([]byte, 0, len(chunk)+len(p))
	out = append(out, p[:i]...)
	out = append(out, chunk...)
	return append(out, p[i:]...)
}

// GIF encodes a solid (w,h)-sized GIF with an explicit palette.
func GIF(w, h int) []byte {
	img := image.NewPaletted(image.Rect(0, 0, w, h),
		color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}})
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// BMP returns a file with a BITMAPINFOHEADER (no pixel data is needed,
// the parser only reads headers). h < 0 encodes a top-down bitmap.
func BMP(w, h int32) []byte {
	fh := make([]byte, 14)
	fh[0], fh[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(fh[2:6], 54) // declared file size
	binary.LittleEndian.PutUint32(fh[10:14], 54)

	dib := new(bytes.Buffer)
	dib.Write(le32(40))         // BITMAPINFOHEADER size
	dib.Write(leI32(w))         // width
	dib.Write(leI32(h))         // height
	dib.Write(le16(1))          // planes
	dib.Write(le16(24))         // bits per pixel
	dib.Write(make([]byte, 24)) // compression, sizeimage, ppm×2, colors

	out := fh
	out = append(out, dib.Bytes()...)
	return out
}

/* ---------- WebP fixtures ---------- */

// WebPVP8 returns a simple lossy WebP with dimensions from the keyframe header.
func WebPVP8(w, h int) []byte {
	payload := []byte{0x9C, 0x01, 0x00, 0x9D, 0x01, 0x2A} // frame tag + start code
	payload = append(payload, le16(uint16(w))...)
	payload = append(payload, le16(uint16(h))...)
	return riff(chunk("VP8 ", payload))
}

// WebPVP8L returns a lossless WebP with 14-bit-dimensions payload.
func WebPVP8L(w, h int) []byte {
	n := uint32(w-1) | uint32(h-1)<<14
	payload := append([]byte{0x2F}, le32(n)...)
	return riff(chunk("VP8L", payload))
}

// WebPVP8X returns an extended WebP with a VP8X canvas and an EXIF chunk.
func WebPVP8X(w, h int, exif []byte) []byte {
	vp8x := []byte{0x10, 0, 0, 0}              // flags + reserved bytes
	vp8x = append(vp8x, leu24(uint32(w-1))...) // canvas width-1
	vp8x = append(vp8x, leu24(uint32(h-1))...) // canvas height-1

	var chunks bytes.Buffer
	chunks.Write(chunk("VP8X", vp8x))
	chunks.Write(chunk("EXIF", exif))
	return riff(chunks.Bytes())
}

func chunk(typ string, payload []byte) []byte {
	c := make([]byte, 8)
	copy(c[:4], typ)
	binary.LittleEndian.PutUint32(c[4:8], uint32(len(payload)))
	c = append(c, payload...)
	if len(payload)%2 == 1 {
		c = append(c, 0) // chunks are padded to even size
	}
	return c
}

func riff(chunks []byte) []byte {
	out := []byte("RIFF")
	out = append(out, le32(uint32(4+len(chunks)))...)
	out = append(out, "WEBP"...)
	return append(out, chunks...)
}

/*
	Phone returns a rich EXIF payload — camera identity, shot settings and

GPS coordinates, as found in a smartphone photo. The exact values are
asserted across all unit tests.
*/
func Phone() []byte {
	return (&TIFF{
		Ifd0: []Entry{
			String(0x010F, "Apple"),               // Make
			String(0x0110, "iPhone 15 Pro"),       // Model
			String(0x0131, "iOS 26.0"),            // Software
			String(0x0132, "2026:07:14 18:32:01"), // DateTime
			Short(0x0112, 6),                      // Orientation: rotate 90° CW
		},
		Exif: []Entry{
			String(0x9003, "2026:07:14 18:32:01"),       // DateTimeOriginal
			Short(0x8827, 125),                          // ISO
			Rats(0x829A, [][2]uint32{{1, 120}}),         // ExposureTime
			Rats(0x829D, [][2]uint32{{89, 50}}),         // FNumber 1.78
			Rats(0x920A, [][2]uint32{{686, 100}}),       // FocalLength 6.86 mm
			Short(0x9209, 1),                            // Flash fired
			Short(0x9307, 5),                            // MeteringMode pattern
			Short(0xA403, 0),                            // WhiteBalance auto
			String(0xA433, "Apple"),                     // LensMake
			String(0xA434, "iPhone 15 Pro back camera"), // LensModel
			Short(0xA405, 26),                           // FocalLength35
		},
		Gps: []Entry{
			String(0x0001, "N"), // LatitudeRef
			Rats(0x0002, [][2]uint32{{55, 1}, {45, 1}, {83, 10}}), // 45′8.3″
			String(0x0003, "E"), // LongitudeRef
			Rats(0x0004, [][2]uint32{{37, 1}, {36, 1}, {5600, 100}}),
			Byte(0x0005, 0),                       // AltitudeRef: above sea
			Rats(0x0006, [][2]uint32{{1515, 10}}), // Altitude 151.5 m
		},
	}).Build()
}

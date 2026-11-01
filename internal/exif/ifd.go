package exif

import (
	"bytes"
	"encoding/binary"
	"strings"
)

// value is one resolved IFD entry: a known value type and its bytes.
// The byte order travels with the value because inline and offset-stored
// data are both encoded in the IFD byte order.
type value struct {
	typ   uint16
	data  []byte
	order binary.ByteOrder
}

// EXIF field type codes.
const (
	typByte      = 1
	typASCII     = 2
	typShort     = 3
	typLong      = 4
	typRational  = 5
	typSLONG     = 9
	typSRational = 10
)

func typeSize(t uint16) int {
	switch t {
	case typByte, typASCII, 6, 7: // BYTE, ASCII, SBYTE, UNDEFINED
		return 1
	case typShort, 8: // SHORT, USHORT
		return 2
	case typLong, typSLONG, 11: // LONG, SLONG, FLOAT
		return 4
	case typRational, typSRational, 12: // RATIONAL, SRATIONAL, DOUBLE
		return 8
	}
	return 0
}

// walkIFD iterates the entries of one IFD. Truncated or absurd entries are
// skipped; out-of-range IFD offsets end the walk. The bounds checks make a
// panic impossible from image data alone (Parse keeps a recover as a
// safety net anyway).
func walkIFD(b []byte, order binary.ByteOrder, off int, cb func(uint16, value)) {
	if off < 2 || off+2 > len(b) || off > len(b)-2 {
		return
	}
	n := int(u16(order, b, off))
	if n > 4096 { // absurd entry count: reject the whole IFD
		return
	}
	for i := 0; i < n; i++ {
		e := off + 2 + i*12
		if e+12 > len(b) {
			return
		}
		tag := u16(order, b, e)
		typ := u16(order, b, e+2)
		cnt := u32(order, b, e+4)
		voff := u32(order, b, e+8)

		sz := typeSize(typ)
		if sz == 0 || cnt == 0 {
			continue
		}
		total := int64(sz) * int64(cnt)
		if total > int64(len(b)) {
			continue
		}

		var data []byte
		switch {
		case total <= 4: // value stored inline in the 4-byte field
			data = b[e+8 : e+8+int(total)]
		default: // value lives at voff
			vo := int64(voff)
			if vo < 2 || vo+total > int64(len(b)) {
				continue
			}
			data = b[vo : vo+total]
		}
		cb(tag, value{typ: typ, data: data, order: order})
	}
}

// rational is the numerator/denominator pair of a RATIONAL field.
// num keeps the sign for SRATIONAL values.
type rational struct {
	num, den int64
}

// u16 reads a uint16, panicking on out-of-bounds (Parse recovers).
func u16(o binary.ByteOrder, b []byte, off int) uint16 {
	if off < 0 || off+2 > len(b) {
		panic("exif: read out of bounds")
	}
	return o.Uint16(b[off : off+2])
}

// u32 reads a uint32, panicking on out-of-bounds.
func u32(o binary.ByteOrder, b []byte, off int) uint32 {
	if off < 0 || off+4 > len(b) {
		panic("exif: read out of bounds")
	}
	return o.Uint32(b[off : off+4])
}

// s32 reads a signed int32, panicking on out-of-bounds.
func s32(o binary.ByteOrder, b []byte, off int) int32 {
	if off < 0 || off+4 > len(b) {
		panic("exif: read out of bounds")
	}
	return int32(o.Uint32(b[off : off+4]))
}

// firstLong decodes the first integer of a BYTE/SHORT/LONG value.
// Covers the common "ISO stored as LONG" and similar quirks.
func (v value) firstLong() uint32 {
	switch v.typ {
	case typByte:
		if len(v.data) >= 1 {
			return uint32(v.data[0])
		}
	case typShort:
		return uint32(u16(v.order, v.data, 0))
	case typLong:
		return u32(v.order, v.data, 0)
	case typSLONG:
		n := s32(v.order, v.data, 0)
		if n < 0 {
			n = -n
		}
		return uint32(n)
	}
	return 0
}

// rationals decodes every RATIONAL/SRATIONAL element of the value.
func (v value) rationals() []rational {
	if v.typ != typRational && v.typ != typSRational {
		return nil
	}
	n := len(v.data) / typeSize(v.typ)
	out := make([]rational, 0, n)
	for i := 0; i < n; i++ {
		off := i * 8
		r := rational{}
		if v.typ == typRational {
			r.num = int64(u32(v.order, v.data, off))
			r.den = int64(u32(v.order, v.data, off+4))
		} else {
			r.num = int64(int32(u32(v.order, v.data, off)))
			r.den = int64(int32(u32(v.order, v.data, off+4)))
		}
		out = append(out, r)
	}
	return out
}

// ascii decodes an ASCII field: stops at the first NUL, keeps only
// printable characters and trims camera noise like spaces or padding.
func (v value) ascii() string {
	b := v.data
	if nul := bytes.IndexByte(b, 0); nul >= 0 {
		b = b[:nul]
	}
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c != 0x7F {
			sb.WriteByte(c)
		}
	}
	return strings.TrimSpace(sb.String())
}

// refLetter returns the first character of a GPS reference tag when it is
// one of N/S/E/W (case-insensitive), otherwise "".
func (v value) refLetter() string {
	s := v.ascii()
	if s == "" {
		return ""
	}
	switch letter := strings.ToUpper(s[:1]); letter {
	case "N", "S", "E", "W":
		return letter
	}
	return ""
}

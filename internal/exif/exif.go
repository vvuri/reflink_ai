// Package exif implements a minimal TIFF/EXIF parser built exclusively on
// the standard library. It reads the metadata payload shared by JPEG APP1
// segments, PNG eXIf chunks and WebP EXIF chunks.
//
// The parser never panics on malformed input: every read is bounds-checked
// and a deferred recover turns any residual bug into an error, so damaged
// metadata degrades to "partial data" instead of crashing the request.
package exif

import (
	"encoding/binary"
	"fmt"
)

// Data carries the EXIF fields the service reports.
type Data struct {
	Present bool // true when at least one known field was read

	Make             string
	Model            string
	LensMake         string
	LensModel        string
	Software         string
	DateTime         string // IFD0:    "2006:01:02 15:04:05"
	DateTimeOriginal string // ExifIFD: "2006:01:02 15:04:05"

	ISO           int
	ExposureTime  float64 // seconds
	FNumber       float64
	FocalLength   float64 // mm
	FocalLength35 int     // 35mm equivalent, 0 unknown
	Flash         int
	MeteringMode  int
	WhiteBalance  int
	Orientation   int // EXIF code 1..8, 0 unknown

	GPS GPS
}

// GPS holds decoded geographic data. Latitude/longitude are already signed:
// S/W hemispheres are negative, N/E positive.
type GPS struct {
	Present       bool    // true when both latitude and longitude were read
	Latitude      float64 // signed decimal degrees
	Longitude     float64 // signed decimal degrees
	HasAltitude   bool
	Altitude      float64 // meters, always positive
	AltitudeBelow bool    // true = below sea level (GPSAltitudeRef == 1)
}

// Known IFD0 tags.
const (
	tagMake        = 0x010F
	tagModel       = 0x0110
	tagOrientation = 0x0112
	tagSoftware    = 0x0131
	tagDateTime    = 0x0132
	tagExifIFD     = 0x8769 // pointer to the Exif sub-IFD
	tagGPSIFD      = 0x8825 // pointer to the GPS sub-IFD
)

// Known Exif sub-IFD tags.
const (
	tagDateTimeOriginal = 0x9003
	tagExposureTime     = 0x829A
	tagFNumber          = 0x829D
	tagISO              = 0x8827
	tagFlash            = 0x9209
	tagFocalLength      = 0x920A
	tagMeteringMode     = 0x9307
	tagWhiteBalance     = 0xA403
	tagLensMake         = 0xA433
	tagLensModel        = 0xA434
	tagFocalLength35    = 0xA405
)

// Known GPS sub-IFD tags.
const (
	gpsLatRef   = 0x0001
	gpsLat      = 0x0002
	gpsLonRef   = 0x0003
	gpsLon      = 0x0004
	gpsAltRef   = 0x0005
	gpsAltitude = 0x0006
)

// Parse decodes the raw TIFF payload of an EXIF stream. Malformed input is
// tolerated: the returned Data keeps everything parsed before the failure
// point and err describes the problem.
func Parse(tiff []byte) (data *Data, err error) {
	data = new(Data)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("exif: malformed payload (%v)", r)
			if !data.Present {
				data = new(Data)
			}
		}
	}()

	if len(tiff) < 8 {
		return data, fmt.Errorf("exif: payload too short")
	}
	var order binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		order = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		order = binary.BigEndian
	default:
		return data, fmt.Errorf("exif: unknown byte order % x", tiff[:2])
	}
	if magic := u16(order, tiff, 2); magic != 42 {
		return data, fmt.Errorf("exif: unexpected header magic %d", magic)
	}

	var exifIFD, gpsIFD int
	var found bool

	walkIFD(tiff, order, int(u32(order, tiff, 4)), func(tag uint16, v value) {
		switch tag {
		case tagMake,
			tagModel,
			tagSoftware,
			tagDateTime,
			tagOrientation:
			assigned := fillIFD0(data, tag, v)
			if assigned {
				found = true
			}
		case tagExifIFD:
			if o := v.firstLong(); o != 0 {
				exifIFD = int(o)
			}
		case tagGPSIFD:
			if o := v.firstLong(); o != 0 {
				gpsIFD = int(o)
			}
		}
	})

	if exifIFD != 0 {
		walkIFD(tiff, order, exifIFD, func(tag uint16, v value) {
			if fillExifIFD(data, tag, v) {
				found = true
			}
		})
	}

	if gpsIFD != 0 {
		g := walkGPS(tiff, order, gpsIFD)
		if g.Present {
			found = true
		}
		data.GPS = g
	}

	data.Present = found
	return data, nil
}

// fillIFD0 maps one IFD0 tag onto Data. Returns true when a field was set.
func fillIFD0(d *Data, tag uint16, v value) bool {
	switch tag {
	case tagMake:
		if s := v.ascii(); s != "" {
			d.Make = s
			return true
		}
	case tagModel:
		if s := v.ascii(); s != "" {
			d.Model = s
			return true
		}
	case tagSoftware:
		if s := v.ascii(); s != "" {
			d.Software = s
			return true
		}
	case tagDateTime:
		if s := v.ascii(); s != "" {
			d.DateTime = s
			return true
		}
	case tagOrientation:
		if n := v.firstLong(); n >= 1 && n <= 8 {
			d.Orientation = int(n)
			return true
		}
	}
	return false
}

// fillExifIFD maps one Exif sub-IFD tag onto Data. Returns true on success.
func fillExifIFD(d *Data, tag uint16, v value) bool {
	switch tag {
	case tagDateTimeOriginal:
		if s := v.ascii(); s != "" {
			d.DateTimeOriginal = s
			return true
		}
	case tagISO:
		if n := v.firstLong(); n != 0 {
			d.ISO = int(n)
			return true
		}
	case tagFlash:
		d.Flash = int(v.firstLong())
		return true
	case tagMeteringMode:
		d.MeteringMode = int(v.firstLong())
		return true
	case tagWhiteBalance:
		d.WhiteBalance = int(v.firstLong())
		return true
	case tagLensMake:
		if s := v.ascii(); s != "" {
			d.LensMake = s
			return true
		}
	case tagLensModel:
		if s := v.ascii(); s != "" {
			d.LensModel = s
			return true
		}
	case tagFocalLength35:
		if n := v.firstLong(); n != 0 {
			d.FocalLength35 = int(n)
			return true
		}
	}

	if f, ok := firstRatio(v); ok {
		ratio := float64(f.num) / float64(f.den)
		switch tag {
		case tagExposureTime:
			d.ExposureTime = ratio
			return true
		case tagFNumber:
			d.FNumber = ratio
			return true
		case tagFocalLength:
			d.FocalLength = ratio
			return true
		}
	}
	return false
}

// firstRatio returns the first rational stored in a RATIONAL/SRATIONAL value.
func firstRatio(v value) (r rational, ok bool) {
	rs := v.rationals()
	if len(rs) == 0 || rs[0].den == 0 {
		return rational{}, false
	}
	return rs[0], true
}

// walkGPS reads the GPS sub-IFD and converts DMS rationals to signed
// decimal degrees using the reference letters (N/S, E/W).
func walkGPS(b []byte, order binary.ByteOrder, ifd int) GPS {
	var (
		g                GPS
		latRef, lonRef   string
		lat, lon, alt    []rational
		haveLat, haveLon bool
	)

	walkIFD(b, order, ifd, func(tag uint16, v value) {
		switch tag {
		case gpsLatRef:
			latRef = v.refLetter()
		case gpsLat:
			lat = v.rationals()
		case gpsLonRef:
			lonRef = v.refLetter()
		case gpsLon:
			lon = v.rationals()
		case gpsAltRef:
			if o := v.firstLong(); o <= 1 {
				g.AltitudeBelow = o == 1
			}
		case gpsAltitude:
			alt = v.rationals()
		}
	})

	if deg, ok := dmsToDegrees(lat); ok && absFloat(deg) <= 90 {
		g.Latitude = signedHemisphere(deg, latRef, 'S')
		haveLat = true
	}
	if deg, ok := dmsToDegrees(lon); ok && absFloat(deg) <= 180 {
		g.Longitude = signedHemisphere(deg, lonRef, 'W')
		haveLon = true
	}
	g.Present = haveLat && haveLon

	// The "0°0'0\" with no fix" trap: both coordinates zero is not a place.
	if g.Present && g.Latitude == 0 && g.Longitude == 0 {
		g.Present = false
	}

	if len(alt) > 0 && alt[0].den != 0 {
		g.Altitude = float64(alt[0].num) / float64(alt[0].den)
		if g.Altitude < 0 { // negative rationals seen in the wild
			g.Altitude, g.AltitudeBelow = -g.Altitude, !g.AltitudeBelow
		}
		g.HasAltitude = true
	}
	return g
}

package youtube

import (
	"encoding/base64"
	"strings"
)

// SearchFilters controls YouTube search filtering via the sp= parameter.
type SearchFilters struct {
	Sort           string // relevance, date, views, rating
	Type           string // video, channel, playlist
	Duration       string // short (<4m), medium (4-20m), long (>20m)
	UploadDate     string // hour, today, week, month, year
	HD             bool
	CC             bool // closed captions / subtitles
	CreativeCommon bool
	Live           bool
	FourK          bool
	ThreeSixty     bool
	HDR            bool
	VR180          bool
}

// IsEmpty reports whether no filter is set.
func (f SearchFilters) IsEmpty() bool {
	return f.Sort == "" && f.Type == "" && f.Duration == "" && f.UploadDate == "" &&
		!f.HD && !f.CC && !f.CreativeCommon && !f.Live && !f.FourK &&
		!f.ThreeSixty && !f.HDR && !f.VR180
}

// Encode returns the base64url-encoded protobuf sp parameter, or "" if empty.
func (f SearchFilters) Encode() string {
	if f.IsEmpty() {
		return ""
	}
	var outer []byte
	if sortVal := sortValue(f.Sort); sortVal > 0 {
		outer = appendVarint(outer, makeTag(1, 0))
		outer = appendVarint(outer, uint64(sortVal))
	}
	var inner []byte
	if v := uploadDateValue(f.UploadDate); v > 0 {
		inner = appendVarint(inner, makeTag(1, 0))
		inner = appendVarint(inner, uint64(v))
	}
	if v := typeValue(f.Type); v > 0 {
		inner = appendVarint(inner, makeTag(2, 0))
		inner = appendVarint(inner, uint64(v))
	}
	if v := durationValue(f.Duration); v > 0 {
		inner = appendVarint(inner, makeTag(3, 0))
		inner = appendVarint(inner, uint64(v))
	}
	if f.HD {
		inner = appendVarint(inner, makeTag(4, 0))
		inner = appendVarint(inner, 1)
	}
	if f.CC {
		inner = appendVarint(inner, makeTag(5, 0))
		inner = appendVarint(inner, 1)
	}
	if f.CreativeCommon {
		inner = appendVarint(inner, makeTag(6, 0))
		inner = appendVarint(inner, 1)
	}
	if f.Live {
		inner = appendVarint(inner, makeTag(8, 0))
		inner = appendVarint(inner, 1)
	}
	if f.FourK {
		inner = appendVarint(inner, makeTag(14, 0))
		inner = appendVarint(inner, 1)
	}
	if f.ThreeSixty {
		inner = appendVarint(inner, makeTag(15, 0))
		inner = appendVarint(inner, 1)
	}
	if f.HDR {
		inner = appendVarint(inner, makeTag(25, 0))
		inner = appendVarint(inner, 1)
	}
	if f.VR180 {
		inner = appendVarint(inner, makeTag(26, 0))
		inner = appendVarint(inner, 1)
	}
	if len(inner) > 0 {
		outer = appendVarint(outer, makeTag(2, 2))
		outer = appendVarint(outer, uint64(len(inner)))
		outer = append(outer, inner...)
	}
	return base64.URLEncoding.EncodeToString(outer)
}

func sortValue(s string) int {
	switch strings.ToLower(s) {
	case "relevance", "":
		return 0
	case "date", "upload_date":
		return 1
	case "views", "view_count":
		return 2
	case "rating":
		return 3
	default:
		return 0
	}
}

func uploadDateValue(s string) int {
	switch strings.ToLower(s) {
	case "hour":
		return 1
	case "today":
		return 2
	case "week":
		return 3
	case "month":
		return 4
	case "year":
		return 5
	default:
		return 0
	}
}

func typeValue(s string) int {
	switch strings.ToLower(s) {
	case "video":
		return 1
	case "channel":
		return 2
	case "playlist":
		return 3
	default:
		return 0
	}
}

func durationValue(s string) int {
	switch strings.ToLower(s) {
	case "short":
		return 1
	case "medium":
		return 2
	case "long":
		return 3
	default:
		return 0
	}
}

func makeTag(field, wireType int) uint64 { return uint64(field<<3 | wireType) }

func appendVarint(buf []byte, v uint64) []byte {
	for v >= 0x80 {
		buf = append(buf, byte(v)|0x80)
		v >>= 7
	}
	return append(buf, byte(v))
}

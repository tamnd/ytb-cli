package youtube

import (
	"fmt"
	"strings"
)

// SubtitleFormat is a target caption serialization.
type SubtitleFormat string

const (
	SubSRT  SubtitleFormat = "srt"
	SubVTT  SubtitleFormat = "vtt"
	SubText SubtitleFormat = "txt"
)

// RenderSubtitles serializes timed segments into srt, vtt, or plain text. The
// conversion is pure-Go and needs no ffmpeg.
func RenderSubtitles(segs []TranscriptSegment, format SubtitleFormat) string {
	switch format {
	case SubVTT:
		return renderVTT(segs)
	case SubText:
		return renderPlainText(segs)
	default:
		return renderSRT(segs)
	}
}

func renderSRT(segs []TranscriptSegment) string {
	var b strings.Builder
	for i, s := range segs {
		end := s.StartSeconds + s.DurSeconds
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n",
			i+1, srtTimestamp(s.StartSeconds), srtTimestamp(end), strings.TrimSpace(s.Text))
	}
	return b.String()
}

func renderVTT(segs []TranscriptSegment) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, s := range segs {
		end := s.StartSeconds + s.DurSeconds
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n",
			vttTimestamp(s.StartSeconds), vttTimestamp(end), strings.TrimSpace(s.Text))
	}
	return b.String()
}

func renderPlainText(segs []TranscriptSegment) string {
	var b strings.Builder
	for _, s := range segs {
		if t := strings.TrimSpace(s.Text); t != "" {
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// srtTimestamp formats seconds as HH:MM:SS,mmm (comma decimal separator).
func srtTimestamp(sec float64) string {
	h, m, s, ms := splitClock(sec)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// vttTimestamp formats seconds as HH:MM:SS.mmm (dot decimal separator).
func vttTimestamp(sec float64) string {
	h, m, s, ms := splitClock(sec)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

func splitClock(sec float64) (h, m, s, ms int) {
	if sec < 0 {
		sec = 0
	}
	total := int(sec * 1000)
	ms = total % 1000
	total /= 1000
	s = total % 60
	total /= 60
	m = total % 60
	h = total / 60
	return h, m, s, ms
}

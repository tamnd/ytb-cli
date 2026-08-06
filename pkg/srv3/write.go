package srv3

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Format is an output serialization.
type Format string

const (
	FormatText Format = "text"
	FormatSRT  Format = "srt"
	FormatVTT  Format = "vtt"
	FormatJSON Format = "json"
)

// Formats lists what Render accepts, for a caller building a usage line.
var Formats = []Format{FormatText, FormatSRT, FormatVTT, FormatJSON}

// Valid reports whether Render accepts f. A command checks this before it
// fetches, so a typo in --format costs nothing and is answered as the usage
// problem it is rather than after a request that is then thrown away.
func (f Format) Valid() bool {
	switch f {
	case FormatText, FormatSRT, FormatVTT, FormatJSON, "txt", "":
		return true
	}
	return false
}

// Render writes the document in one of the four formats.
//
// All four come off the same parsed cues, which is the point of parsing once:
// the srt and the vtt and the json cannot drift apart, and the text is the same
// text the subtitles carry rather than a second extraction of it.
func (d *Document) Render(f Format) (string, error) {
	switch f {
	case FormatText, "txt", "":
		return d.Text(), nil
	case FormatSRT:
		return d.SRT(), nil
	case FormatVTT:
		return d.VTT(), nil
	case FormatJSON:
		return d.JSON()
	default:
		return "", fmt.Errorf("unknown transcript format %q: want text, srt, vtt or json", f)
	}
}

// Text is the transcript as prose, one cue per line.
func (d *Document) Text() string {
	var b strings.Builder
	for _, c := range d.Cues {
		b.WriteString(c.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// SRT writes SubRip.
func (d *Document) SRT() string {
	var b strings.Builder
	for i, c := range d.timed() {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1,
			stamp(c.StartMS, ','), stamp(c.EndMS, ','), c.Text)
	}
	return b.String()
}

// VTT writes WebVTT.
func (d *Document) VTT() string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, c := range d.timed() {
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", stamp(c.StartMS, '.'), stamp(c.EndMS, '.'), c.Text)
	}
	return b.String()
}

// JSON writes the whole document, head and word timings included.
func (d *Document) JSON() (string, error) {
	out, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out) + "\n", nil
}

// line is a cue with its end resolved.
type line struct {
	StartMS int
	EndMS   int
	Text    string
}

// timed resolves each cue's end for a subtitle writer.
//
// The stated durations on an auto track overlap, and by a lot: on dQw4w9WgXcQ
// the cue at 18800 states 7160ms, which runs to 25960, while the next cue starts
// at 21800. That is honest about the screen, where a rollup line stays up while
// the next is typed under it, and it is wrong for a subtitle file, where two
// overlapping cues are drawn on top of each other. So an end is clamped to the
// next cue's start, and only a cue that would otherwise have no duration at all
// is given the stated one.
func (d *Document) timed() []line {
	out := make([]line, 0, len(d.Cues))
	for i, c := range d.Cues {
		end := c.EndMS()
		if i+1 < len(d.Cues) {
			if next := d.Cues[i+1].StartMS; next > c.StartMS && next < end {
				end = next
			}
		}
		if end <= c.StartMS {
			end = c.StartMS + c.DurMS
		}
		if end <= c.StartMS {
			// A cue with no duration stated and nothing after it still has to be on
			// screen long enough to read.
			end = c.StartMS + 2000
		}
		out = append(out, line{StartMS: c.StartMS, EndMS: end, Text: c.Text})
	}
	return out
}

// stamp formats milliseconds as HH:MM:SS<sep>mmm. srt separates with a comma and
// vtt with a dot, which is the only difference between the two timestamps.
func stamp(ms int, sep byte) string {
	if ms < 0 {
		ms = 0
	}
	msec := ms % 1000
	sec := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", sec/3600, sec/60%60, sec%60, sep, msec)
}

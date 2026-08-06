// Package srv3 parses YouTube's srv3 timed text and writes it back out as text,
// srt, vtt or json.
//
// srv3 is what the site itself asks for. A caption baseUrl off the ANDROID
// player already ends in &fmt=srv3, so this is the native format and not a
// conversion, and everything else the endpoint serves is a lossy view of it.
// The older &fmt-less response is <transcript><text start= dur=> with the text
// double escaped, so "&#39;" arrives as "&amp;#39;" and a reader that unescapes
// once ships the entities. json3 is the same document with longer key names.
//
// The file is parsed once into a Document and every writer works off that, so
// the srt and the vtt and the json cannot disagree about where a line starts.
//
// Two things in the format will produce nonsense if they are not handled, and
// both are in the auto-generated tracks that most videos only have:
//
//	<w>          a window definition, in the body next to the cues, with no text
//	<p a="1">    a rollup event: the same window redrawn, holding only a newline
//
// On the auto track of dQw4w9WgXcQ the body holds 104 nodes: 1 window, 51
// rollups and 52 cues. Counting all 104 as lines gives a transcript that is half
// blank and an srt with 51 empty subtitles in it.
package srv3

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// Document is one parsed timed text file.
//
// The head is kept even though no writer here draws a caption on a video,
// because it is what the file says about itself and dropping it would make this
// a text extractor rather than a parser.
type Document struct {
	// Format is the version the file declares, "3" for srv3.
	Format string `json:"format,omitempty"`
	// Cues are the lines that carry text, in file order.
	Cues []Cue `json:"cues"`
	// Windows are the caption boxes the cues are drawn into.
	Windows []Window `json:"windows,omitempty"`
	// WindowStyles and WindowPositions are the head's <ws> and <wp> definitions.
	WindowStyles    []WindowStyle    `json:"window_styles,omitempty"`
	WindowPositions []WindowPosition `json:"window_positions,omitempty"`
	Pens            []Pen            `json:"pens,omitempty"`
	// Rollups counts the append events dropped, which is how a caller can tell an
	// auto-generated track from a human one without asking the player.
	Rollups int `json:"rollups"`
}

// Cue is one line of timed text.
type Cue struct {
	StartMS int `json:"start_ms"`
	// DurMS is what the file states. On an auto track the states overlap, because
	// a rollup caption stays on screen while the next one is typed underneath it,
	// so EndMS and not this is what a subtitle writer should use.
	DurMS    int `json:"dur_ms"`
	WindowID int `json:"window_id,omitempty"`
	// Text is the whole line, with the words joined as the file spaces them.
	Text string `json:"text"`
	// Words is the per-word timing an auto track carries and a human one does not.
	Words []Word `json:"words,omitempty"`
}

// EndMS is where the line stops, before any clamp against the next one.
func (c Cue) EndMS() int { return c.StartMS + c.DurMS }

// Word is one timed piece of a cue.
type Word struct {
	Text string `json:"text"`
	// OffsetMS is from the cue's start, which is how the file states it. The first
	// word of a cue carries no offset and is therefore 0.
	OffsetMS int `json:"offset_ms"`
	// ASRConfidence is the recogniser's own confidence, present only on an auto
	// track. It is a pointer because 0 is a value the field really takes.
	ASRConfidence *int `json:"asr_confidence,omitempty"`
}

// StartMS is the word's own start on the video's clock.
func (w Word) StartMS(cue Cue) int { return cue.StartMS + w.OffsetMS }

// Window is a caption box declared in the body.
type Window struct {
	ID         int `json:"id"`
	StartMS    int `json:"start_ms"`
	StyleID    int `json:"style_id,omitempty"`
	PositionID int `json:"position_id,omitempty"`
}

// WindowStyle is a <ws> from the head. The names come from json3, which is the
// same document written out with the fields spelled: mh is mhModeHint, ju is
// juJustifCode, sd is sdScrollDir.
type WindowStyle struct {
	ID        int `json:"id"`
	ModeHint  int `json:"mode_hint,omitempty"`
	Justify   int `json:"justify,omitempty"`
	ScrollDir int `json:"scroll_dir,omitempty"`
}

// WindowPosition is a <wp> from the head: apPoint, ahHorPos, avVerPos, rcRows
// and ccCols in json3's spelling.
type WindowPosition struct {
	ID      int `json:"id"`
	Point   int `json:"anchor_point,omitempty"`
	HorPos  int `json:"hor_pos,omitempty"`
	VerPos  int `json:"ver_pos,omitempty"`
	Rows    int `json:"rows,omitempty"`
	Columns int `json:"columns,omitempty"`
}

// Pen is a <pen> from the head, the colour and face a run is drawn with.
type Pen struct {
	ID         int    `json:"id"`
	FontSize   int    `json:"font_size,omitempty"`
	FontStyle  int    `json:"font_style,omitempty"`
	Foreground string `json:"foreground,omitempty"`
	Background string `json:"background,omitempty"`
	Bold       bool   `json:"bold,omitempty"`
	Italic     bool   `json:"italic,omitempty"`
	Underline  bool   `json:"underline,omitempty"`
}

// ErrEmpty is returned for a body with no bytes in it.
//
// It has its own error because the caption endpoint answers a request it will
// not serve with HTTP 200 and zero bytes rather than with a status: the watch
// page's own baseUrl does it every time, and so does a good URL with the
// language changed. That is a refusal and it is worth saying so, because
// "no transcript for this video" is a different thing and the video usually has
// one.
var ErrEmpty = errors.New("empty timed text body: the endpoint answered 200 with no bytes, which is a refusal and not an absent transcript")

// xml mirrors of the file, kept unexported so the shape on the wire and the
// shape callers use can move independently.
type xmlDoc struct {
	XMLName xml.Name `xml:"timedtext"`
	Format  string   `xml:"format,attr"`
	Head    xmlHead  `xml:"head"`
	Body    xmlBody  `xml:"body"`
}

type xmlHead struct {
	WindowStyles    []xmlWS  `xml:"ws"`
	WindowPositions []xmlWP  `xml:"wp"`
	Pens            []xmlPen `xml:"pen"`
}

type xmlWS struct {
	ID        int `xml:"id,attr"`
	ModeHint  int `xml:"mh,attr"`
	Justify   int `xml:"ju,attr"`
	ScrollDir int `xml:"sd,attr"`
}

type xmlWP struct {
	ID      int `xml:"id,attr"`
	Point   int `xml:"ap,attr"`
	HorPos  int `xml:"ah,attr"`
	VerPos  int `xml:"av,attr"`
	Rows    int `xml:"rc,attr"`
	Columns int `xml:"cc,attr"`
}

type xmlPen struct {
	ID         int    `xml:"id,attr"`
	FontSize   int    `xml:"sz,attr"`
	FontStyle  int    `xml:"fs,attr"`
	Foreground string `xml:"fc,attr"`
	Background string `xml:"bc,attr"`
	Bold       int    `xml:"b,attr"`
	Italic     int    `xml:"i,attr"`
	Underline  int    `xml:"u,attr"`
}

type xmlBody struct {
	Windows []xmlW `xml:"w"`
	Paras   []xmlP `xml:"p"`
}

type xmlW struct {
	ID       int `xml:"id,attr"`
	StartMS  int `xml:"t,attr"`
	Position int `xml:"wp,attr"`
	Style    int `xml:"ws,attr"`
}

type xmlP struct {
	StartMS  int    `xml:"t,attr"`
	DurMS    int    `xml:"d,attr"`
	WindowID int    `xml:"w,attr"`
	Append   int    `xml:"a,attr"`
	Chardata string `xml:",chardata"`
	Segs     []xmlS `xml:"s"`
}

type xmlS struct {
	OffsetMS int    `xml:"t,attr"`
	ASRConf  *int   `xml:"ac,attr"`
	Text     string `xml:",chardata"`
}

// Parse reads an srv3 document.
//
// It is deliberately strict about nothing except emptiness. YouTube serves this
// file with a stated encoding and well-formed markup, and a body that will not
// parse is a signal worth passing up rather than working around.
func Parse(data []byte) (*Document, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, ErrEmpty
	}
	var x xmlDoc
	if err := xml.Unmarshal(data, &x); err != nil {
		return nil, fmt.Errorf("parse srv3: %w", err)
	}

	doc := &Document{Format: x.Format}
	for _, ws := range x.Head.WindowStyles {
		doc.WindowStyles = append(doc.WindowStyles, WindowStyle{
			ID: ws.ID, ModeHint: ws.ModeHint, Justify: ws.Justify, ScrollDir: ws.ScrollDir,
		})
	}
	for _, wp := range x.Head.WindowPositions {
		doc.WindowPositions = append(doc.WindowPositions, WindowPosition{
			ID: wp.ID, Point: wp.Point, HorPos: wp.HorPos, VerPos: wp.VerPos,
			Rows: wp.Rows, Columns: wp.Columns,
		})
	}
	for _, p := range x.Head.Pens {
		doc.Pens = append(doc.Pens, Pen{
			ID: p.ID, FontSize: p.FontSize, FontStyle: p.FontStyle,
			Foreground: p.Foreground, Background: p.Background,
			Bold: p.Bold == 1, Italic: p.Italic == 1, Underline: p.Underline == 1,
		})
	}
	// A window is declared in the body, in among the cues, and a reader that takes
	// every body child for a line counts it as one with no text on it.
	for _, w := range x.Body.Windows {
		doc.Windows = append(doc.Windows, Window{
			ID: w.ID, StartMS: w.StartMS, StyleID: w.Style, PositionID: w.Position,
		})
	}

	for _, p := range x.Body.Paras {
		// a="1" is a rollup: the window redrawn one line further up, carrying a
		// newline and nothing else. It is a thing the player does to the screen and
		// not a thing the speaker said.
		if p.Append == 1 {
			doc.Rollups++
			continue
		}
		cue := Cue{StartMS: p.StartMS, DurMS: p.DurMS, WindowID: p.WindowID}
		if len(p.Segs) > 0 {
			var b strings.Builder
			for _, s := range p.Segs {
				if s.Text == "" {
					continue
				}
				b.WriteString(s.Text)
				cue.Words = append(cue.Words, Word{
					Text: s.Text, OffsetMS: s.OffsetMS, ASRConfidence: s.ASRConf,
				})
			}
			cue.Text = b.String()
		} else {
			cue.Text = p.Chardata
		}
		// A newline inside a cue is the line break the caption was drawn with. It is
		// kept out of the text and the words are joined with a space, because a
		// transcript is prose and the break was a decision about a 40 column box.
		cue.Text = collapse(cue.Text)
		if cue.Text == "" {
			continue
		}
		doc.Cues = append(doc.Cues, cue)
	}
	return doc, nil
}

// collapse turns the caption's own line breaks and padding into single spaces.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

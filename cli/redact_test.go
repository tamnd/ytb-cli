package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit/render"
	"github.com/tamnd/ytb-cli/ytb"
)

// redact_test.go is the output half of one rule: a cookie value never leaves the
// session file. The disk half is in ytb/session_test.go, which walks the data
// directory after a signed-in read.
//
// It is worth a test rather than a review because the leak is one careless field
// away. The obvious way to render a session is to marshal it, and a marshalled
// Session prints the cookies into whatever somebody piped ytb into. That is why
// SessionStatus is a separate type, and this is what keeps it one.
//
// Every format is checked, not one. A row carries both rendered columns and the
// value behind them, -o json and -o csv reach different halves of it, and a leak
// in the half a test skipped is a leak.

const secret = "SAPISID-VALUE-THAT-MUST-NEVER-APPEAR-ANYWHERE"

// formats is every way ytb can print a record.
var formats = []render.Format{
	render.List, render.Table, render.Markdown, render.JSON,
	render.JSONL, render.CSV, render.TSV, render.URL,
}

func renderRow(t *testing.T, f render.Format, r Row) string {
	t.Helper()
	var buf bytes.Buffer
	out, err := render.New(render.Options{Format: f, Writer: &buf})
	if err != nil {
		t.Fatalf("render.New(%s): %v", f, err)
	}
	if err := out.Emit(r); err != nil {
		t.Fatalf("emit as %s: %v", f, err)
	}
	if err := out.Flush(); err != nil {
		t.Fatalf("flush as %s: %v", f, err)
	}
	return buf.String()
}

func TestAuthStatusNeverPrintsACookie(t *testing.T) {
	dir := t.TempDir()
	s := ytb.ParseCookies("SID=sid-value; HSID=hsid-value; SAPISID=" + secret)
	if err := ytb.SaveSession(dir, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := ytb.LoadSession(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	row := sessionRow(loaded.Status(ytb.SessionPath(dir)))

	for _, f := range formats {
		got := renderRow(t, f, row)
		if strings.Contains(got, secret) {
			t.Errorf("ytb auth status prints a cookie as %s:\n%s", f, got)
		}
		// The names are the point of the command, so they have to survive the
		// thing that hides the values. -o url is the exception and prints
		// nothing at all here, because the row has no url column.
		if f == render.URL {
			continue
		}
		if !strings.Contains(got, "SAPISID") {
			t.Errorf("ytb auth status as %s hides the cookie names it exists to print:\n%s", f, got)
		}
	}
}

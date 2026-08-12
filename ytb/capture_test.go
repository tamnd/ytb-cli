package ytb

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func archiveClient(t *testing.T, cache bool) *Client {
	t.Helper()
	c := NewClient(Config{})
	if cache {
		c.SetCache(NewCache(t.TempDir(), time.Minute))
	}
	return c
}

// The capture and the parse have to see the same bytes, so with the cache off
// there is nowhere for the capture to come from and the command says so instead
// of fetching the page twice.
func TestArchiveRefusesWithTheCacheOff(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cap")
	_, err := Archive(context.Background(), archiveClient(t, false), "dQw4w9WgXcQ", dir, ClaimOptions{})
	if !errors.Is(err, ErrCacheOff) {
		t.Fatalf("err = %v, want ErrCacheOff", err)
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		t.Error("a refused archive left a directory behind")
	}
}

// A read ytb cannot make a record of still archives, with the reason in
// record.json. That is the case this command exists for: the report that says
// the parser missed something arrives with the bytes attached.
func TestArchiveWritesTheParseError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cap")
	// A mix id is generated per viewer and browses to nothing, so this refusal
	// happens before any request goes out and the test stays offline.
	cap, err := Archive(context.Background(), archiveClient(t, true), "RDdQw4w9WgXcQ", dir, ClaimOptions{})
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if cap.ParseError == "" {
		t.Error("the capture reports no parse error on a reference nothing can read")
	}

	raw, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		t.Fatalf("read record.json: %v", err)
	}
	var rec struct {
		Ref     string `json:"ref"`
		Error   string `json:"error"`
		Records []any  `json:"records"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal record.json: %v", err)
	}
	if rec.Error == "" {
		t.Error("record.json does not say why the read produced nothing")
	}
	if rec.Ref != "RDdQw4w9WgXcQ" {
		t.Errorf("ref = %q, want the reference that was asked for", rec.Ref)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta.json")); err != nil {
		t.Errorf("meta.json is missing: %v", err)
	}
}

// A session header is replaced with a note rather than written down, so an
// archive is safe to attach to a bug report.
func TestArchiveMetaHasNoSession(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cap")
	if _, err := Archive(context.Background(), archiveClient(t, true), "RDdQw4w9WgXcQ", dir, ClaimOptions{}); err != nil {
		t.Fatalf("archive: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}
	for _, header := range []string{`"Cookie": "`, `"Authorization": "`} {
		if i := strings.Index(string(raw), header); i >= 0 {
			rest := string(raw)[i+len(header):]
			if !strings.HasPrefix(rest, "(removed") {
				t.Errorf("meta.json wrote a %s header out in full", header)
			}
		}
	}
}

func TestEndpointName(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/youtubei/v1/browse?prettyPrint=false#browse VLPL123": "browse",
		"https://www.youtube.com/youtubei/v1/player":                                  "player",
		"https://music.youtube.com/youtubei/v1/next#next dQw4w9WgXcQ":                 "next",
		"": "innertube",
	}
	for in, want := range cases {
		if got := endpointName(in); got != want {
			t.Errorf("endpointName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The file is named after what is in it, so a captured feed is not called
// page.html and a JSONP answer is not called page.xml.
func TestPageExtension(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"<!DOCTYPE html><html>", ".html"},
		{`<?xml version="1.0"?><feed>`, ".xml"},
		{`{"responseContext":{}}`, ".json"},
		{"  <transcript><text/></transcript>", ".xml"},
	}
	for _, c := range cases {
		if got := pageExtension(Read{Body: []byte(c.body)}); got != c.want {
			t.Errorf("pageExtension(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

// A payload goes in innertube/ and a page does not, and the answer comes from
// the body rather than the URL because the feed is XML from an address that
// looks like every other GET.
func TestIsJSONPayload(t *testing.T) {
	if !isJSONPayload(Read{Method: "POST", Body: []byte(`{"contents":{}}`)}) {
		t.Error("a browse payload was not recognised")
	}
	if isJSONPayload(Read{Method: "GET", Body: []byte(`{"contents":{}}`)}) {
		t.Error("a GET was filed as an InnerTube payload")
	}
	if isJSONPayload(Read{Method: "POST", Body: []byte("<html>")}) {
		t.Error("an HTML body was filed as an InnerTube payload")
	}
}

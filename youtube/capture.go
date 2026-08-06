package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tamnd/ytb-cli/pkg/graph"
)

// capture.go is ytb archive. Spec 3005 doc 04 section 4.1.
//
// It writes one read down in full: the page, every InnerTube payload, the
// headers as they were sent, and what ytb made of all of it. This is how a bug
// report should be filed. A report saying the playlist parser missed the
// continuation is a conversation; the same report with this directory attached
// is a fixture.

// ErrCacheOff is what archive says when it has nowhere to put the bytes.
var ErrCacheOff = errors.New("the cache is off and ytb archive needs it on")

// Capture is what one archive run wrote.
type Capture struct {
	Ref   string   `json:"ref"`
	Dir   string   `json:"dir" kit:"id"`
	Files []string `json:"files"`
	Reads int      `json:"reads"`
	Bytes int      `json:"bytes"`
	// ParseError is filled when ytb could not make a record of what it fetched.
	// The capture still happened, and this is the case the command is most useful
	// for: the bytes are on disk beside the reason they defeated the parser.
	ParseError string `json:"parse_error,omitempty"`
}

// archiveMeta is meta.json: the requests as they were made.
type archiveMeta struct {
	Ref       string        `json:"ref"`
	Tool      string        `json:"tool"`
	FetchedAt string        `json:"fetched_at"`
	Reads     []archiveRead `json:"reads"`
}

// archiveRead is one request with the file its answer went into, because a
// directory holding page.html and two browse payloads does not otherwise say
// which request produced which file.
type archiveRead struct {
	File string `json:"file,omitempty"`
	Read
}

// archiveRecord is record.json: what ytb parsed out of the bytes next to it.
type archiveRecord struct {
	Ref     string       `json:"ref"`
	Error   string       `json:"error,omitempty"`
	Records []any        `json:"records"`
	Claims  []graph.Edge `json:"claims"`
	Nodes   []graph.URI  `json:"nodes"`
}

// Archive fetches ref for real and writes everything about it into dir.
//
// It never answers from the cache, because the point is what YouTube is serving
// now. It does write what it fetched into the cache, which is why it refuses to
// run with the cache off: the bytes on disk and the bytes ytb parsed have to be
// the same bytes or the capture is a different read from the one it describes.
func Archive(ctx context.Context, c *Client, ref, dir string, opt ClaimOptions) (*Capture, error) {
	if !c.Cache().Enabled() {
		return nil, fmt.Errorf("%w: the bytes on disk and the bytes ytb parsed have to be the same bytes", ErrCacheOff)
	}
	if dir == "" {
		return nil, errors.New("archive needs a directory to write into")
	}
	if err := os.MkdirAll(filepath.Join(dir, "innertube"), 0o755); err != nil {
		return nil, err
	}

	var reads []Read
	c.SetOnRead(func(r Read) { reads = append(reads, r) })
	c.Cache().SetBypass(true)
	defer func() {
		c.SetOnRead(nil)
		c.Cache().SetBypass(false)
	}()

	col := NewCollector()
	parseErr := c.Collect(ctx, ref, opt, col)

	cap := &Capture{Ref: ref, Dir: dir, Reads: len(reads)}
	if parseErr != nil {
		cap.ParseError = parseErr.Error()
	}

	logged := make([]archiveRead, 0, len(reads))
	pages, payloads := 0, map[string]int{}
	for _, r := range reads {
		cap.Bytes += r.Bytes
		if len(r.Body) == 0 {
			logged = append(logged, archiveRead{Read: r})
			continue
		}
		var name string
		if isJSONPayload(r) {
			endpoint := endpointName(r.URL)
			client := strings.ToLower(r.Client)
			if client == "" {
				client = "web"
			}
			base := endpoint + "-" + client
			payloads[base]++
			if n := payloads[base]; n > 1 {
				base = fmt.Sprintf("%s-%d", base, n)
			}
			name = filepath.Join("innertube", base+".json")
		} else {
			pages++
			base := "page"
			if pages > 1 {
				base = fmt.Sprintf("page-%d", pages)
			}
			name = base + pageExtension(r)
		}
		if err := os.WriteFile(filepath.Join(dir, name), r.Body, 0o644); err != nil {
			return cap, err
		}
		cap.Files = append(cap.Files, name)
		logged = append(logged, archiveRead{File: name, Read: r})
	}

	meta := archiveMeta{
		Ref:       ref,
		Tool:      "ytb",
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Reads:     logged,
	}
	if err := writeJSON(filepath.Join(dir, "meta.json"), meta); err != nil {
		return cap, err
	}
	cap.Files = append(cap.Files, "meta.json")

	rec := archiveRecord{
		Ref:     ref,
		Records: col.Records,
		Claims:  col.Set.Edges(),
		Nodes:   col.Set.Nodes(),
	}
	if parseErr != nil {
		rec.Error = parseErr.Error()
	}
	if rec.Records == nil {
		rec.Records = []any{}
	}
	if err := writeJSON(filepath.Join(dir, "record.json"), rec); err != nil {
		return cap, err
	}
	cap.Files = append(cap.Files, "record.json")
	return cap, nil
}

// isJSONPayload says whether a read goes in innertube/ rather than beside the
// page. It asks the body rather than the URL, because the feed is XML served
// from an address that looks like every other GET.
func isJSONPayload(r Read) bool {
	if r.Method != "POST" {
		return false
	}
	body := strings.TrimSpace(string(r.Body))
	return strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[")
}

// pageExtension names the file after what is in it, so a captured Atom feed is
// not called page.html.
func pageExtension(r Read) string {
	body := strings.TrimSpace(string(r.Body))
	switch {
	case strings.HasPrefix(body, "{"), strings.HasPrefix(body, "["):
		return ".json"
	case strings.HasPrefix(body, "<?xml"), strings.HasPrefix(body, "<feed"), strings.HasPrefix(body, "<transcript"), strings.HasPrefix(body, "<timedtext"):
		return ".xml"
	default:
		return ".html"
	}
}

// endpointName is the InnerTube verb out of a logged URL: browse, player, next,
// search. The operation the reads log wrote on as a fragment is dropped here,
// because it is already in meta.json and it is not a filename.
func endpointName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "innertube"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	name := parts[len(parts)-1]
	if name == "" {
		return "innertube"
	}
	return name
}

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	// The whole point is a file somebody reads and pastes into an issue, so the
	// escaping stays off: a description with a > in it should look like itself.
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

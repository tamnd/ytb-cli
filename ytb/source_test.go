package ytb

import "testing"

// Every record in this tool is supposed to name the URL it was read from, and
// four readers did not: search, trending, related and hashtag all handed back
// rows with an empty sources list. A row you cannot trace back to a page you can
// open is the one thing the envelope exists to prevent.
//
// These are the URL builders rather than the readers, because the readers need
// the network. What broke was never the parse, it was that nobody stamped the
// URL, so the URL is what is worth pinning down.
func TestASearchRowNamesAPageYouCanOpen(t *testing.T) {
	cases := []struct {
		name  string
		query string
		f     SearchFilters
		want  string
	}{
		{
			name:  "no filters",
			query: "lofi hip hop",
			want:  "https://www.youtube.com/results?search_query=lofi+hip+hop",
		},
		{
			// Two searches for the same words with different filters are different
			// reads, so they had better not claim the same source.
			name:  "the filter blob rides along",
			query: "lofi",
			f:     SearchFilters{Type: "video"},
			want:  "https://www.youtube.com/results?search_query=lofi&sp=EgIQAQ%3D%3D",
		},
		{
			name:  "a query that needs escaping",
			query: "rock & roll",
			want:  "https://www.youtube.com/results?search_query=rock+%26+roll",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := searchResultsURL(tc.query, tc.f); got != tc.want {
				t.Errorf("searchResultsURL(%q) = %q, want %q", tc.query, got, tc.want)
			}
		})
	}
}

// A suggestion was the last record with no envelope on it, on the grounds that
// there is nothing to qualify about a bare string. There is: which surface
// answered, and which locale the guess was conditioned on, both of which are in
// the URL. s9 is also the only surface a suggestion can name, so a suggestion
// claiming s2 would mean the read went somewhere else entirely.
func TestASuggestionSaysWhereItCameFrom(t *testing.T) {
	const source = "https://suggestqueries-clients6.youtube.com/complete/search?client=youtube&ds=yt&hl=en&gl=US&q=lofi"
	s := NewSuggestion("lofi hip hop", source)

	if s.Text != "lofi hip hop" {
		t.Errorf("text = %q", s.Text)
	}
	if s.Kind != "suggestion" {
		t.Errorf("kind = %q, want suggestion", s.Kind)
	}
	if !s.Surfaces.Has(SurfaceSuggest) || len(s.Surfaces) != 1 {
		t.Errorf("surfaces = %v, want just %s", s.Surfaces, SurfaceSuggest)
	}
	if len(s.Sources) != 1 || s.Sources[0] != source {
		t.Errorf("sources = %v, want the request that was made", s.Sources)
	}
	if s.Tier != 0 {
		t.Errorf("tier = %d, want 0: autocomplete never carries cookies", s.Tier)
	}
	// The autocomplete endpoint is JSONP with no client context, so there is no
	// client to name. Empty is the honest answer and nil would not be, because
	// nil reads as nobody having kept track.
	if s.Client == nil || len(s.Client) != 0 {
		t.Errorf("client = %v, want an empty list: no InnerTube client is claimed", s.Client)
	}
}

// addSource is what actually fills the field, and it has to stay idempotent:
// the related shelf and the watch page name the same URL, and a record that
// listed it twice would read as two reads where there was one.
func TestASourceIsRecordedOnce(t *testing.T) {
	v := NewVideo("dQw4w9WgXcQ", SurfaceWatchHTML)
	v.addSource(NormalizeVideoURL("dQw4w9WgXcQ"))
	v.addSource(NormalizeVideoURL("dQw4w9WgXcQ"))
	if len(v.Sources) != 1 {
		t.Fatalf("sources = %v, want the one URL", v.Sources)
	}
	if v.Sources[0] != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Errorf("sources = %v", v.Sources)
	}
}

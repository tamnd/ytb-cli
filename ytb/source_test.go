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

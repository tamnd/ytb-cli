package ytb

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// invariants_test.go holds the tables to their own rules. Doc 06 section 5.
//
// A table is a promise that a set is closed: eleven surfaces, two tiers, five
// clients, twenty-one predicates. The promise is worth something only while the
// code agrees with it, and the way a table stops agreeing is never dramatic. A
// surface gets a constant and no read ever claims it. A twelfth id appears in one
// parser as a literal. A client is added to the list and nothing calls it. None of
// that fails a build and none of it fails a parse, so a record ends up with an
// evidence list that is quietly incomplete, which is worse than one that is
// obviously wrong.

// surfaceIDs is doc 01's table, id to the constant's name, in the order the doc
// numbers them.
var surfaceIDs = map[string]string{
	"s1":  "SurfaceWatchHTML",
	"s2":  "SurfaceInnerTube",
	"s3":  "SurfaceMobilePlayer",
	"s4":  "SurfaceBrowseHTML",
	"s5":  "SurfaceOEmbed",
	"s6":  "SurfaceFeed",
	"s7":  "SurfaceThumbCDN",
	"s8":  "SurfaceMediaCDN",
	"s9":  "SurfaceSuggest",
	"s10": "SurfaceMusic",
	"s11": "SurfaceSession",
}

// TestSurfaceTableIsClosed asserts the eleven ids are eleven, that they are the
// eleven the doc names, and that the constants carry the values this test says
// they do.
func TestSurfaceTableIsClosed(t *testing.T) {
	got := map[string]string{
		SurfaceWatchHTML:    "SurfaceWatchHTML",
		SurfaceInnerTube:    "SurfaceInnerTube",
		SurfaceMobilePlayer: "SurfaceMobilePlayer",
		SurfaceBrowseHTML:   "SurfaceBrowseHTML",
		SurfaceOEmbed:       "SurfaceOEmbed",
		SurfaceFeed:         "SurfaceFeed",
		SurfaceThumbCDN:     "SurfaceThumbCDN",
		SurfaceMediaCDN:     "SurfaceMediaCDN",
		SurfaceSuggest:      "SurfaceSuggest",
		SurfaceMusic:        "SurfaceMusic",
		SurfaceSession:      "SurfaceSession",
	}
	// Eleven distinct constants have to produce eleven distinct ids. Two sharing
	// one is the copy-paste that makes a record claim the wrong evidence.
	if len(got) != 11 {
		t.Fatalf("the eleven surface constants collapse to %d distinct ids, so two of them share a value", len(got))
	}
	for id, name := range surfaceIDs {
		if got[id] != name {
			t.Errorf("%s is %s in doc 01 and %s here", id, name, got[id])
		}
	}
	for id := range got {
		if _, ok := surfaceIDs[id]; !ok {
			t.Errorf("surface id %q is in the code and not in doc 01's table", id)
		}
	}
}

// TestEverySurfaceIsClaimedBySomeRead asserts each id is named outside the file
// that declares it.
//
// A surface nothing claims is a row in the doc that describes a read the tool
// does not do. That is a documentation bug rather than a crash, and it is the
// kind that survives for years because nobody goes looking for the absence of a
// string.
func TestEverySurfaceIsClaimedBySomeRead(t *testing.T) {
	claims := map[string]int{}
	for _, path := range repoFiles(t) {
		base := filepath.Base(path)
		if strings.HasSuffix(path, "_test.go") || base == "envelope.go" {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		for _, name := range surfaceIDs {
			// The constant, not the id: a literal "s2" in a parser is the thing the
			// constants exist to prevent, and TestNoBareSurfaceLiteral catches that.
			claims[name] += strings.Count(text, name)
		}
	}
	for id, name := range surfaceIDs {
		if claims[name] == 0 {
			t.Errorf("%s (%s) is declared and no read ever claims it, so doc 01 describes a surface this tool does not use", name, id)
		}
	}
}

// TestNoBareSurfaceLiteral asserts a surface reaches an envelope as a constant.
//
// "s3" written out is a claim nothing checks: the compiler is happy, the JSON
// looks right, and a renumbering leaves one parser pointing at a surface that
// moved. The constants are the whole reason a typo is a build failure.
func TestNoBareSurfaceLiteral(t *testing.T) {
	for _, path := range repoFiles(t) {
		base := filepath.Base(path)
		if strings.HasSuffix(path, "_test.go") || base == "envelope.go" {
			continue
		}
		for lit, pos := range stringLiterals(t, path) {
			if len(lit) < 2 || lit[0] != 's' {
				continue
			}
			if _, err := strconv.Atoi(lit[1:]); err != nil {
				continue
			}
			if _, ok := surfaceIDs[lit]; ok {
				t.Errorf("%s: the surface id %q is written out. Use the %s constant, "+
					"which is what makes a typo a build failure.", pos, lit, surfaceIDs[lit])
			}
		}
	}
}

// TestSurfaceForURLOnlyReturnsTableIDs asserts the address-to-surface mapping
// stays inside the table, for every address this tool actually fetches.
//
// The mapping is by substring because the reads log is written below the forty
// call sites and none of them knows what surface it is on. That is the right
// trade and it has one failure mode: a new address falls through to the default
// and gets logged as a browse page, or a new case returns an id nobody declared.
func TestSurfaceForURLOnlyReturnsTableIDs(t *testing.T) {
	cases := []struct {
		url, client, want string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "", SurfaceWatchHTML},
		{"https://youtu.be/dQw4w9WgXcQ", "", SurfaceWatchHTML},
		{"https://www.youtube.com/shorts/Df5Y-2ndQyU", "", SurfaceWatchHTML},
		{"https://www.youtube.com/youtubei/v1/browse?key=x", "WEB", SurfaceInnerTube},
		{"https://www.youtube.com/youtubei/v1/player?key=x", "WEB", SurfaceInnerTube},
		{"https://www.youtube.com/youtubei/v1/player?key=x", "ANDROID", SurfaceMobilePlayer},
		{"https://www.youtube.com/@RickAstleyYT", "", SurfaceBrowseHTML},
		{"https://www.youtube.com/oembed?url=x", "", SurfaceOEmbed},
		{"https://www.youtube.com/feeds/videos.xml?channel_id=UC", "", SurfaceFeed},
		{"https://i.ytimg.com/vi/dQw4w9WgXcQ/maxresdefault.jpg", "", SurfaceThumbCDN},
		{"https://rr3---sn-abc.googlevideo.com/videoplayback?expire=1", "", SurfaceMediaCDN},
		{"https://suggestqueries-clients6.youtube.com/complete/search?q=x", "", SurfaceSuggest},
		{"https://music.youtube.com/youtubei/v1/browse?key=x", "WEB_REMIX", SurfaceMusic},
	}
	for _, tc := range cases {
		got := surfaceForURL(tc.url, tc.client)
		if got != tc.want {
			t.Errorf("surfaceForURL(%q, %q) = %s (%s), want %s (%s)",
				tc.url, tc.client, got, surfaceIDs[got], tc.want, surfaceIDs[tc.want])
		}
		if _, ok := surfaceIDs[got]; !ok {
			t.Errorf("surfaceForURL(%q) returned %q, which is not a surface", tc.url, got)
		}
	}
	// music.youtube.com wins over the player rule, because a WEB_REMIX player call
	// is a music read and not a mobile one. This is the ordering in the switch and
	// it is easy to break by adding a case above it.
	if got := surfaceForURL("https://music.youtube.com/youtubei/v1/player", "ANDROID"); got != SurfaceMusic {
		t.Errorf("a music player call is %s, want %s", got, SurfaceMusic)
	}
}

// TestTierTableIsTwoRows asserts the only way up is a session.
//
// Doc 01 section 12: tier 0 is what anyone can reproduce, tier 1 is what this
// machine's cookies could see. The number is on the record so a dataset built
// with cookies is distinguishable from one built without, and the whole claim
// rests on nothing else being able to raise it.
func TestTierTableIsTwoRows(t *testing.T) {
	anon := NewClient(DefaultConfig())
	if anon.Tier() != 0 {
		t.Errorf("a client with no cookies is tier %d", anon.Tier())
	}
	v := NewVideo("dQw4w9WgXcQ", SurfaceWatchHTML)
	anon.stamp(v)
	if v.Tier != 0 || v.Surfaces.Has(SurfaceSession) {
		t.Errorf("a signed out read produced tier %d with surfaces %v", v.Tier, v.Surfaces)
	}

	signed := NewClient(DefaultConfig())
	// The two that make a session a session, which is what Session.missing checks.
	signed.SetSession(ParseCookies("SID=sid-value; SAPISID=sapisid-value; HSID=x"))
	if signed.Tier() != 1 {
		t.Fatalf("a client with cookies is tier %d, and there are only two tiers", signed.Tier())
	}
	signed.stamp(v)
	if v.Tier != 1 {
		t.Errorf("a session read left the record at tier %d", v.Tier)
	}
	if !v.Surfaces.Has(SurfaceSession) {
		t.Errorf("a session read did not add %s, so the record does not say what made it tier 1", SurfaceSession)
	}

	// A tier is never given back. One authenticated call is enough to make the
	// whole record unreproducible signed out, so a later anonymous read that
	// contributed a field must not lower it.
	anon.stamp(v)
	if v.Tier != 1 {
		t.Errorf("an anonymous read dropped the record from tier 1 to %d", v.Tier)
	}

	// And an empty session is not a session. A caller hands over whatever
	// LoadSession returned without checking it, and the tier has to be honest.
	empty := NewClient(DefaultConfig())
	empty.SetSession(ParseCookies(""))
	if empty.Tier() != 0 {
		t.Errorf("an empty session claims tier %d", empty.Tier())
	}
}

// TestClientTableNamesRealClients asserts every spec in the table is one this
// tool calls, and that the names in it are the names on the wire.
//
// The other half of the clients table, that a spec's headers and its context
// agree, is in policy_test.go. This half is the cheaper question nobody asks: is
// there a client here that nothing uses. A spec that is never called is a set of
// version numbers that goes stale silently, and the day somebody reaches for it
// they get an UNPLAYABLE they cannot explain.
func TestClientTableNamesRealClients(t *testing.T) {
	used := map[string]int{}
	for _, path := range repoFiles(t) {
		base := filepath.Base(path)
		if strings.HasSuffix(path, "_test.go") || base == "clients.go" {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		for _, s := range Clients() {
			// The constructor rather than the name, because the name appears in
			// comments and in the reads log and neither is a call. The underscore
			// comes out because WEB_REMIX on the wire is ClientWEBREMIX in Go.
			used[s.Name] += strings.Count(text, "Client"+strings.ReplaceAll(s.Name, "_", "")+"()")
		}
	}
	for _, s := range Clients() {
		if used[s.Name] == 0 {
			t.Errorf("Client%s() is in the table and nothing calls it. A spec nobody uses goes stale "+
				"without anyone finding out, and the version numbers in it are the part that matters.",
				strings.ReplaceAll(s.Name, "_", ""))
		}
	}
}

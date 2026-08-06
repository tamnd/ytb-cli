package youtube

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tamnd/ytb-cli/pkg/graph"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "ytb.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// A node a claim named and nobody fetched is a row with no record, and that is
// what the frontier is made of.
func TestStoreSightsUnreadNodes(t *testing.T) {
	s := testStore(t)

	uri := graph.VideoURI("dQw4w9WgXcQ")
	if err := s.Sight(uri); err != nil {
		t.Fatalf("sight: %v", err)
	}
	n, err := s.Node(uri)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if n == nil {
		t.Fatal("sighted node is not in the store")
	}
	if n.Read() {
		t.Error("a sighted node reports itself as read")
	}
	if n.Kind != graph.Video {
		t.Errorf("kind = %q, want video", n.Kind)
	}

	front, err := s.Frontier("", 10)
	if err != nil {
		t.Fatalf("frontier: %v", err)
	}
	if len(front) != 1 || front[0] != uri {
		t.Errorf("frontier = %v, want just %s", front, uri)
	}
}

// A caption track has no address of its own, so nothing will ever fetch one and
// it does not belong on the frontier.
func TestStoreSkipsFragments(t *testing.T) {
	s := testStore(t)

	frag := graph.URI(string(graph.VideoURI("dQw4w9WgXcQ")) + "#captions/en")
	if _, ok := graph.Parse(frag); !ok {
		t.Skip("the fragment spelling changed, so this test is checking nothing")
	}
	if err := s.Sight(frag); err != nil {
		t.Fatalf("sight: %v", err)
	}
	front, err := s.Frontier("", 10)
	if err != nil {
		t.Fatalf("frontier: %v", err)
	}
	if len(front) != 0 {
		t.Errorf("frontier = %v, want nothing: a fragment is not a node to fetch", front)
	}
}

// The rule from doc 04: a node met twice keeps the better sighting, and a later
// sighting with no record does not erase a record that is there.
func TestStoreRecordSurvivesLaterSighting(t *testing.T) {
	s := testStore(t)

	uri, err := s.PutRecord(Video{VideoID: "dQw4w9WgXcQ", Title: "Never Gonna Give You Up"})
	if err != nil {
		t.Fatalf("put record: %v", err)
	}
	if err := s.Sight(uri); err != nil {
		t.Fatalf("sight: %v", err)
	}

	n, err := s.Node(uri)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if !n.Read() {
		t.Fatal("a later sighting blanked the record")
	}
	var v Video
	if err := json.Unmarshal(n.Record, &v); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if v.Title != "Never Gonna Give You Up" {
		t.Errorf("title = %q, want the stored one", v.Title)
	}

	front, err := s.Frontier(graph.Video, 10)
	if err != nil {
		t.Fatalf("frontier: %v", err)
	}
	if len(front) != 0 {
		t.Errorf("frontier = %v, want nothing: the video has been read", front)
	}
}

// A Song names a video node, so filing one there would put the music app's view
// of a track where the video record goes.
func TestStoreRefusesRecordItCannotPlace(t *testing.T) {
	s := testStore(t)
	if _, err := s.PutRecord(Song{VideoID: "dQw4w9WgXcQ", Title: "Never Gonna Give You Up"}); err == nil {
		t.Fatal("a Song was filed as a node record")
	}
}

// The claims key is the triple plus the source plus the client, so the same edge
// seen by two clients is two observations and seen twice by one is one.
func TestStoreClaimsKeyedByClient(t *testing.T) {
	s := testStore(t)

	video := graph.VideoURI("dQw4w9WgXcQ")
	channel := graph.ChannelURI("UCuAXFkgsw1L7xaCfnd5JJOw")
	base := graph.Edge{
		From:      video,
		Predicate: graph.Published,
		To:        channel,
		Source:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Surface:   SurfaceWatchHTML,
	}
	web := base
	web.Client = "WEB"
	android := base
	android.Client = "ANDROID"

	n, err := s.PutClaims([]graph.Edge{web, android})
	if err != nil {
		t.Fatalf("put claims: %v", err)
	}
	if n != 2 {
		t.Errorf("wrote %d claims, want 2: two clients are two observations", n)
	}

	// The same read again is the same observation, so nothing new is written and
	// nothing is duplicated.
	if _, err := s.PutClaims([]graph.Edge{web}); err != nil {
		t.Fatalf("put claims again: %v", err)
	}
	got, err := s.Claims(video, graph.Published, "", 0)
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("claims = %d rows, want 2", len(got))
	}

	// Both ends came along as nodes, which is what keeps the frontier from being
	// empty on a store full of claims.
	if node, err := s.Node(channel); err != nil || node == nil {
		t.Fatalf("the channel a claim named is not in the store (%v)", err)
	}
}

// A note is a label rather than part of the claim, so a later sighting that
// carries one fills in an earlier one that did not.
func TestStoreClaimNoteFilledIn(t *testing.T) {
	s := testStore(t)

	e := graph.Edge{
		From:      graph.PlaylistURI("PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf"),
		Predicate: graph.Contains,
		To:        graph.VideoURI("dQw4w9WgXcQ"),
		Source:    "https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf",
		Surface:   SurfaceInnerTube,
		Client:    "WEB",
	}
	if _, err := s.PutClaims([]graph.Edge{e}); err != nil {
		t.Fatalf("put claims: %v", err)
	}
	labelled := e
	labelled.Note = "Never Gonna Give You Up"
	labelled.Position = 3
	if _, err := s.PutClaims([]graph.Edge{labelled}); err != nil {
		t.Fatalf("put claims again: %v", err)
	}

	got, err := s.Claims(e.From, "", "", 0)
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("claims = %d rows, want 1: the same client on the same source is one observation", len(got))
	}
	if got[0].Note != "Never Gonna Give You Up" || got[0].Position != 3 {
		t.Errorf("note = %q position = %d, want the later sighting's label", got[0].Note, got[0].Position)
	}
}

// The reads table is the audit log, and stats is how a crawl is read back.
func TestStoreStats(t *testing.T) {
	s := testStore(t)

	if _, err := s.PutRecord(Channel{ChannelID: "UCuAXFkgsw1L7xaCfnd5JJOw", Title: "Rick Astley"}); err != nil {
		t.Fatalf("put record: %v", err)
	}
	if err := s.Sight(graph.VideoURI("dQw4w9WgXcQ")); err != nil {
		t.Fatalf("sight: %v", err)
	}
	if err := s.PutRead(Read{
		URL: "https://www.youtube.com/youtubei/v1/browse#UUuAXFkgsw1L7xaCfnd5JJOw",
		// The operation rides as a fragment because thirty browses of one URL in a
		// log that cannot tell them apart is a log nobody can check a record
		// against, and a fragment is never sent to a server.
		Surface: SurfaceInnerTube, Client: "WEB", Status: 200, Bytes: 4096, At: time.Now(),
	}); err != nil {
		t.Fatalf("put read: %v", err)
	}

	stats, err := s.Stats()
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	want := map[string]bool{
		"nodes:channel":          false,
		"nodes:video (not read)": false,
	}
	sawRead := false
	for _, row := range stats {
		if _, ok := want[row.Table+":"+row.Key]; ok {
			want[row.Table+":"+row.Key] = true
		}
		if row.Table == "reads" && row.Bytes == 4096 {
			sawRead = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("stats has no %s row", k)
		}
	}
	if !sawRead {
		t.Error("stats did not report the bytes the read downloaded")
	}
}

// ytb query opens the file mode=ro, so a statement that writes is refused by
// SQLite rather than by a check in the tool.
func TestStoreReadOnlyRefusesWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ytb.db")
	rw, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := rw.Sight(graph.VideoURI("dQw4w9WgXcQ")); err != nil {
		t.Fatalf("sight: %v", err)
	}
	if err := rw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	ro, err := OpenStoreReadOnly(path)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer func() { _ = ro.Close() }()

	if _, _, err := ro.Query(`SELECT uri FROM nodes`); err != nil {
		t.Fatalf("read-only select: %v", err)
	}
	if _, _, err := ro.Query(`DELETE FROM nodes`); err == nil {
		t.Fatal("a delete went through on a read-only handle")
	}
}

// A file written by the ytb that had a table per record type is refused by name,
// rather than failing three commands later with "no such table: nodes".
func TestStoreRefusesOldFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := old.db.Exec(`CREATE TABLE videos (video_id TEXT PRIMARY KEY, title TEXT)`); err != nil {
		t.Fatalf("fake an old file: %v", err)
	}
	if err := old.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := OpenStore(path); err == nil {
		t.Fatal("an old store opened as if nothing had changed")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the refused file is gone: %v", err)
	}
}

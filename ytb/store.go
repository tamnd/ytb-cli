package ytb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/pkg/ytid"

	_ "modernc.org/sqlite"
)

// store.go is the graph on disk. Spec 3005 doc 04 section 4.
//
// Three tables and no more. An earlier ytb had a table per record type, which
// meant a new kind of thing was a migration, a video nobody had fetched had
// nowhere to live, and the answer to "what have I not looked at yet" was a
// query nobody could write. Nodes and claims replace all of it: a node is
// something with a URI, its record is the last read of it or null when nobody
// has read it, and a claim is one observation of one edge.
//
// The claims key is the triple plus the source plus the client, because the same
// endpoint answers differently depending on which app ytb said it was. Two rows
// differing only by client are two observations and collapsing them would lose
// the fact that only one of the two carries anything.

// Store is the SQLite file. One file, opened read-write by everything that
// crawls and read-only by everything that asks.
type Store struct {
	db       *sql.DB
	path     string
	readOnly bool
}

// storeSchema is the whole thing. Doc 04 section 4 gives the three tables; the
// indexes and the two extra columns on claims are explained where they appear.
const storeSchema = `
CREATE TABLE IF NOT EXISTS nodes (
	uri        TEXT PRIMARY KEY,
	kind       TEXT NOT NULL,
	record     JSON,
	first_seen INTEGER NOT NULL,
	last_seen  INTEGER NOT NULL
);

-- The frontier query, which is the one a crawl runs on every hop: everything of
-- a kind that nothing has read.
CREATE INDEX IF NOT EXISTS nodes_unread ON nodes(kind) WHERE record IS NULL;

CREATE TABLE IF NOT EXISTS claims (
	from_uri  TEXT NOT NULL,
	predicate TEXT NOT NULL,
	to_uri    TEXT NOT NULL,
	source    TEXT NOT NULL,
	surface   TEXT NOT NULL DEFAULT '',
	client    TEXT NOT NULL DEFAULT '',
	tier      INTEGER NOT NULL DEFAULT 0,
	-- note and position are not in the doc's SQL block and are kept anyway. The
	-- note is the title the lockup carried, and on a node nobody has fetched it is
	-- the only human-readable thing in the store about that node. The position is
	-- what a playlist's order rides on.
	note      TEXT NOT NULL DEFAULT '',
	position  INTEGER NOT NULL DEFAULT 0,
	seen_at   INTEGER NOT NULL,
	PRIMARY KEY (from_uri, predicate, to_uri, source, client)
);

CREATE INDEX IF NOT EXISTS claims_to ON claims(to_uri, predicate);
CREATE INDEX IF NOT EXISTS claims_predicate ON claims(predicate);

CREATE TABLE IF NOT EXISTS reads (
	url     TEXT NOT NULL,
	surface TEXT NOT NULL DEFAULT '',
	client  TEXT NOT NULL DEFAULT '',
	status  INTEGER NOT NULL DEFAULT 0,
	bytes   INTEGER NOT NULL DEFAULT 0,
	at      INTEGER NOT NULL,
	error   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS reads_at ON reads(at);
`

// storeTables is what Reset drops. It is spelled out rather than read out of
// sqlite_master so a reset never drops a table something else put in the file.
var storeTables = []string{"nodes", "claims", "reads"}

// ErrOldStore is what an .db written by a ytb before the graph store gets.
// Migrating it is not worth writing: the old file held records with no
// provenance on them, and a claim without its source is not a claim this tool
// would have written.
var ErrOldStore = errors.New("this store was written by an older ytb and its schema is gone")

// OpenStore opens the file read-write, creating it and its directory.
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.checkAge(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(storeSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return s, nil
}

// OpenStoreReadOnly opens the file with mode=ro, which is what ytb query uses.
//
// The point is that a finger slip that says delete is refused by SQLite rather
// than by a check in this tool. A check here would be one regular expression
// away from being wrong, and the database has the answer already.
func OpenStoreReadOnly(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no store at %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	s := &Store{db: db, path: path, readOnly: true}
	if err := s.checkAge(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// checkAge refuses a file from the record-per-table days by name, because the
// alternative is a confusing "no such table: nodes" from three commands down.
func (s *Store) checkAge() error {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='videos'`).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%s: %w, so delete it and crawl again", s.path, ErrOldStore)
}

// Path is the file. Vacuum, Close and Reset are what they say.
func (s *Store) Path() string { return s.path }

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}

func (s *Store) Reset() error {
	for _, t := range storeTables {
		if _, err := s.db.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return fmt.Errorf("drop %s: %w", t, err)
		}
	}
	_, err := s.db.Exec(storeSchema)
	return err
}

// --- nodes ---

// StoredNode is a row of the nodes table. Record is nil on a node something
// named and nothing has read, which is most of them.
type StoredNode struct {
	URI       graph.URI       `json:"uri" kit:"id" table:"uri"`
	Kind      graph.Kind      `json:"kind" table:"kind"`
	Record    json.RawMessage `json:"record,omitempty" table:"-"`
	FirstSeen time.Time       `json:"first_seen" table:"-"`
	LastSeen  time.Time       `json:"last_seen" table:"last_seen"`
}

// Read reports whether anything has actually fetched this node.
func (n StoredNode) Read() bool { return len(n.Record) > 0 }

// Sight records that a claim named this node, without claiming to have read it.
//
// This is the half of the store that makes a crawl resumable. One watch page
// names a channel, thirty related videos and a handful of links, and every one
// of those is something ytb has heard of and not looked at.
func (s *Store) Sight(uri graph.URI) error {
	p, ok := graph.Parse(uri)
	if !ok || p.IsFragment() {
		// A fragment is a part of a node rather than a node: a caption track and a
		// chapter have no address and nothing will ever fetch one on its own.
		return nil
	}
	return s.putNode(uri, p.Kind, nil)
}

// PutRecord writes what a read returned, under the URI the record names.
//
// It takes the record itself rather than a URI and a blob so the caller cannot
// file a channel under a video's URI, and it returns the URI it used so the
// caller can say what it stored.
func (s *Store) PutRecord(record any) (graph.URI, error) {
	uri, kind := recordURI(record)
	if uri == "" {
		return "", fmt.Errorf("%T names no node, so there is nowhere to put it", record)
	}
	blob, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return uri, s.putNode(uri, kind, blob)
}

// putNode is the upsert both of those end in.
//
// A node met twice keeps the better sighting. A later sighting with no record
// does not erase a record that is there, which is what stops a related shelf
// naming a video ytb read last week from blanking it, and last_seen moves
// whichever sighting carried a record.
func (s *Store) putNode(uri graph.URI, kind graph.Kind, record []byte) error {
	now := time.Now().Unix()
	var blob any
	if len(record) > 0 {
		blob = string(record)
	}
	_, err := s.db.Exec(`
		INSERT INTO nodes (uri, kind, record, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(uri) DO UPDATE SET
			kind       = CASE WHEN excluded.kind <> '' THEN excluded.kind ELSE nodes.kind END,
			record     = COALESCE(excluded.record, nodes.record),
			first_seen = MIN(nodes.first_seen, excluded.first_seen),
			last_seen  = MAX(nodes.last_seen, excluded.last_seen)`,
		string(uri), string(kind), blob, now, now)
	if err != nil {
		return fmt.Errorf("put node %s: %w", uri, err)
	}
	return nil
}

// recordURI says where a record belongs. It is the one place that maps a Go
// type to a node, and a type missing from it is a compile-time-invisible bug, so
// every caller checks the empty URI.
func recordURI(record any) (graph.URI, graph.Kind) {
	switch r := record.(type) {
	case Video:
		return graph.VideoURI(r.VideoID), graph.Video
	case *Video:
		return graph.VideoURI(r.VideoID), graph.Video
	case Channel:
		return graph.ChannelURI(r.ChannelID), graph.Channel
	case *Channel:
		return graph.ChannelURI(r.ChannelID), graph.Channel
	case Playlist:
		return graph.PlaylistURI(r.PlaylistID), graph.Playlist
	case *Playlist:
		return graph.PlaylistURI(r.PlaylistID), graph.Playlist
	case Comment:
		return graph.CommentURI(r.ID), graph.Comment
	case CommunityPost:
		return graph.PostURI(r.PostID), graph.Post
	case Album:
		return graph.AlbumURI(r.AlbumID), graph.Album
	case Artist:
		// An artist with a UC id is the channel, doc 04 section 2, and the channel
		// node holds the channel record. Filing the music app's view of the artist
		// there would put a record of one shape under a node of another kind and
		// flip both on every read, so it is not filed and the claims carry it.
		if ytid.IsChannel(r.ArtistID) {
			return "", ""
		}
		return graph.ArtistURI(r.ArtistID), graph.Artist
	// A Track is deliberately not here. It names a video node, and filing one
	// under that node would put the music app's view of a track where the video
	// record goes, so the two would take turns overwriting each other depending on
	// which read ran last.
	default:
		return "", ""
	}
}

// Node reads one node back, record and all.
func (s *Store) Node(uri graph.URI) (*StoredNode, error) {
	row := s.db.QueryRow(`SELECT uri, kind, record, first_seen, last_seen FROM nodes WHERE uri = ?`, string(uri))
	n, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return n, err
}

// Frontier is what a crawl reads next: nodes of a kind that nothing has fetched,
// oldest sighting first so a walk does not keep rediscovering the same corner.
//
// An empty kind means every kind, and only the kinds ytb can actually read come
// back. A hashtag and an external URL are named by claims and are not reads.
func (s *Store) Frontier(kind graph.Kind, limit int) ([]graph.URI, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT uri FROM nodes WHERE record IS NULL`
	args := []any{}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, string(kind))
	} else {
		q += ` AND kind IN ('video','channel','playlist','album','artist')`
	}
	q += ` ORDER BY first_seen, uri LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []graph.URI
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return out, err
		}
		out = append(out, graph.URI(uri))
	}
	return out, rows.Err()
}

// Nodes lists nodes of a kind, read or not, for anything that wants to page
// through the store without writing SQL.
func (s *Store) Nodes(kind graph.Kind, limit int) ([]StoredNode, error) {
	q := `SELECT uri, kind, record, first_seen, last_seen FROM nodes`
	args := []any{}
	if kind != "" {
		q += ` WHERE kind = ?`
		args = append(args, string(kind))
	}
	q += ` ORDER BY uri`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []StoredNode
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return out, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// scanner is what Node and Nodes have in common: sql.Row and sql.Rows both scan.
type scanner interface{ Scan(dest ...any) error }

func scanNode(sc scanner) (*StoredNode, error) {
	var (
		uri, kind string
		record    []byte
		first     int64
		last      int64
	)
	if err := sc.Scan(&uri, &kind, &record, &first, &last); err != nil {
		return nil, err
	}
	return &StoredNode{
		URI:       graph.URI(uri),
		Kind:      graph.Kind(kind),
		Record:    record,
		FirstSeen: time.Unix(first, 0).UTC(),
		LastSeen:  time.Unix(last, 0).UTC(),
	}, nil
}

// --- claims ---

// PutClaims writes a set of claims and every node they named, in one
// transaction, and returns how many rows the claims table gained.
//
// The nodes come along because that is the whole point of the plane: a claim
// naming a video is a video the store now knows exists. Writing the claim
// without the node would leave the frontier empty on a store full of claims.
func (s *Store) PutClaims(edges []graph.Edge) (int, error) {
	if len(edges) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	claim, err := tx.Prepare(`
		INSERT INTO claims (from_uri, predicate, to_uri, source, surface, client, tier, note, position, seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(from_uri, predicate, to_uri, source, client) DO UPDATE SET
			surface  = excluded.surface,
			tier     = excluded.tier,
			-- A later sighting with a note beats an earlier one without, the same
			-- rule graph.Set follows in memory. A note is a label rather than part of
			-- the claim, so it is the one column an observation may overwrite.
			note     = CASE WHEN excluded.note <> '' THEN excluded.note ELSE claims.note END,
			position = CASE WHEN excluded.position <> 0 THEN excluded.position ELSE claims.position END,
			seen_at  = excluded.seen_at`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = claim.Close() }()

	node, err := tx.Prepare(`
		INSERT INTO nodes (uri, kind, record, first_seen, last_seen)
		VALUES (?, ?, NULL, ?, ?)
		ON CONFLICT(uri) DO UPDATE SET last_seen = MAX(nodes.last_seen, excluded.last_seen)`)
	if err != nil {
		return 0, err
	}
	defer func() { _ = node.Close() }()

	now := time.Now().Unix()
	seen := map[graph.URI]bool{}
	written := 0
	for _, e := range edges {
		for _, end := range []graph.URI{e.From, e.To} {
			if seen[end] {
				continue
			}
			seen[end] = true
			p, ok := graph.Parse(end)
			if !ok || p.IsFragment() {
				continue
			}
			if _, err := node.Exec(string(end), string(p.Kind), now, now); err != nil {
				return written, fmt.Errorf("put node %s: %w", end, err)
			}
		}
		res, err := claim.Exec(
			string(e.From), string(e.Predicate), string(e.To),
			e.Source, e.Surface, e.Client, e.Tier, e.Note, e.Position, now)
		if err != nil {
			return written, fmt.Errorf("put claim %s %s %s: %w", e.From, e.Predicate, e.To, err)
		}
		if n, err := res.RowsAffected(); err == nil && n > 0 {
			written++
		}
	}
	return written, tx.Commit()
}

// Claims reads claims back, filtered by whichever ends the caller gave.
func (s *Store) Claims(from graph.URI, p graph.Predicate, to graph.URI, limit int) ([]graph.Edge, error) {
	q := `SELECT from_uri, predicate, to_uri, source, surface, client, tier, note, position FROM claims WHERE 1=1`
	var args []any
	if from != "" {
		q += ` AND from_uri = ?`
		args = append(args, string(from))
	}
	if p != "" {
		q += ` AND predicate = ?`
		args = append(args, string(p))
	}
	if to != "" {
		q += ` AND to_uri = ?`
		args = append(args, string(to))
	}
	q += ` ORDER BY from_uri, predicate, position, to_uri, source, client`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []graph.Edge
	for rows.Next() {
		var e graph.Edge
		if err := rows.Scan(&e.From, &e.Predicate, &e.To, &e.Source, &e.Surface, &e.Client, &e.Tier, &e.Note, &e.Position); err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- reads ---

// PutRead appends one request to the audit log.
//
// This is what makes a record's sources checkable afterwards. A claim says a URL
// asserted it; this table says that URL was fetched, when, as which client, and
// what came back, which is the difference between a provenance field and a
// provenance field somebody can verify.
func (s *Store) PutRead(r Read) error {
	at := r.At
	if at.IsZero() {
		at = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO reads (url, surface, client, status, bytes, at, error) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.URL, r.Surface, r.Client, r.Status, r.Bytes, at.Unix(), r.Error)
	return err
}

// --- stats and query ---

// StatRow is one line of ytb db stats: which table, which bucket, how many.
type StatRow struct {
	Table string `json:"table" table:"table"`
	Key   string `json:"key" kit:"id" table:"key"`
	Rows  int64  `json:"rows" table:"rows"`
	// Bytes is filled on the reads rows only, because how much a crawl downloaded
	// is the number that says whether a budget was spent on watch pages or on
	// feeds.
	Bytes int64 `json:"bytes,omitempty" table:"bytes"`
}

// Stats is nodes by kind, claims by predicate, and reads by surface, client and
// status, which is the fastest way to see that a crawl was mostly 400s.
func (s *Store) Stats() ([]StatRow, error) {
	var out []StatRow

	rows, err := s.db.Query(`
		SELECT kind, COUNT(*), SUM(CASE WHEN record IS NULL THEN 1 ELSE 0 END)
		FROM nodes GROUP BY kind ORDER BY kind`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind string
		var total, unread int64
		if err := rows.Scan(&kind, &total, &unread); err != nil {
			_ = rows.Close()
			return out, err
		}
		out = append(out, StatRow{Table: "nodes", Key: kind, Rows: total})
		if unread > 0 {
			// The unread count is on its own row rather than in a column, because it
			// is the frontier and it is the number a crawl is deciding on.
			out = append(out, StatRow{Table: "nodes", Key: kind + " (not read)", Rows: unread})
		}
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}

	rows, err = s.db.Query(`SELECT predicate, COUNT(*) FROM claims GROUP BY predicate ORDER BY COUNT(*) DESC, predicate`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var p string
		var n int64
		if err := rows.Scan(&p, &n); err != nil {
			_ = rows.Close()
			return out, err
		}
		out = append(out, StatRow{Table: "claims", Key: p, Rows: n})
	}
	if err := closeRows(rows); err != nil {
		return out, err
	}

	rows, err = s.db.Query(`
		SELECT surface, client, status, COUNT(*), SUM(bytes)
		FROM reads GROUP BY surface, client, status ORDER BY COUNT(*) DESC, surface, client, status`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var surface, client string
		var status, n, bytes int64
		if err := rows.Scan(&surface, &client, &status, &n, &bytes); err != nil {
			_ = rows.Close()
			return out, err
		}
		key := surface
		if client != "" {
			key += " " + client
		}
		out = append(out, StatRow{Table: "reads", Key: fmt.Sprintf("%s %d", key, status), Rows: n, Bytes: bytes})
	}
	return out, closeRows(rows)
}

func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

// Query runs the caller's SQL and returns the columns and the rows.
//
// There is no wrapper and no query builder on purpose. The schema is three
// tables a person can hold in their head, and the useful questions are ones
// nobody would have thought to add a flag for.
func (s *Store) Query(sqlText string) ([]string, [][]any, error) {
	rows, err := s.db.Query(sqlText)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return cols, out, err
		}
		out = append(out, vals)
	}
	return cols, out, rows.Err()
}

// --- search ---

// SearchVideos looks for a phrase in the title and description of every video
// record in the store. It is a LIKE over the JSON rather than an index: the
// store is one file on one machine and a few thousand records, and an FTS table
// that has to be kept in step with the records is a second thing to get wrong.
func (s *Store) SearchVideos(q string, limit int) ([]Video, error) {
	return searchRecords[Video](s, graph.Video, q, limit,
		`json_extract(record,'$.title')`, `json_extract(record,'$.description')`)
}

// SearchChannels is the same over channels, with the handle in the net.
func (s *Store) SearchChannels(q string, limit int) ([]Channel, error) {
	return searchRecords[Channel](s, graph.Channel, q, limit,
		`json_extract(record,'$.title')`, `json_extract(record,'$.description')`, `json_extract(record,'$.handle')`)
}

func searchRecords[T any](s *Store, kind graph.Kind, q string, limit int, fields ...string) ([]T, error) {
	if limit <= 0 {
		limit = 20
	}
	pat := "%" + strings.ToLower(q) + "%"
	where := make([]string, 0, len(fields))
	args := []any{string(kind)}
	for _, f := range fields {
		where = append(where, "lower(COALESCE("+f+",'')) LIKE ?")
		args = append(args, pat)
	}
	// The first field ranks: a phrase in the title beats the same phrase buried in
	// a description, which is what a person searching for a title expects.
	args = append(args, pat, limit)
	rows, err := s.db.Query(`
		SELECT record FROM nodes
		WHERE kind = ? AND record IS NOT NULL AND (`+strings.Join(where, " OR ")+`)
		ORDER BY CASE WHEN lower(COALESCE(`+fields[0]+`,'')) LIKE ? THEN 0 ELSE 1 END, uri
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return out, err
		}
		var rec T
		if err := json.Unmarshal(blob, &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// --- read helpers used by export.go ---
//
// The Markdown exporter wants channels, their videos and their playlists, which
// used to be a table each and is now a query each. A record that is not there is
// not an error: the exporter writes what the store has read, and a video the
// store has only heard of has nothing to write a page from.

func (s *Store) storeGetChannel(idOrHandle string) (*Channel, error) {
	if uri := graph.ChannelURI(idOrHandle); uri != "" {
		n, err := s.Node(uri)
		if err != nil {
			return nil, err
		}
		if n != nil && n.Read() {
			return unmarshalRecord[Channel](n.Record)
		}
	}
	handle := strings.TrimPrefix(strings.TrimSpace(idOrHandle), "@")
	var blob []byte
	err := s.db.QueryRow(`
		SELECT record FROM nodes
		WHERE kind = 'channel' AND record IS NOT NULL
		  AND lower(COALESCE(json_extract(record,'$.handle'),'')) IN (?, ?)
		LIMIT 1`, strings.ToLower(handle), strings.ToLower("@"+handle)).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("channel %q is not in the store", idOrHandle)
	}
	if err != nil {
		return nil, err
	}
	return unmarshalRecord[Channel](blob)
}

func (s *Store) storeGetAllChannels() ([]Channel, error) {
	return recordsOfKind[Channel](s, graph.Channel, "")
}

func (s *Store) storeGetVideosByChannel(channelID, channelName string) ([]Video, error) {
	videos, err := recordsOfKind[Video](s, graph.Video, channelID)
	if err != nil {
		return nil, err
	}
	for i := range videos {
		if videos[i].ChannelTitle == "" {
			videos[i].ChannelTitle = channelName
		}
	}
	sort.SliceStable(videos, func(i, j int) bool {
		return videos[i].PublishedAt.After(videos[j].PublishedAt)
	})
	return videos, nil
}

func (s *Store) storeGetPlaylistsByChannel(channelID, channelName string) ([]Playlist, error) {
	playlists, err := recordsOfKind[Playlist](s, graph.Playlist, channelID)
	if err != nil {
		return nil, err
	}
	for i := range playlists {
		if playlists[i].ChannelTitle == "" {
			playlists[i].ChannelTitle = channelName
		}
	}
	return playlists, nil
}

// recordsOfKind returns every record of a kind, optionally only the ones whose
// record names a channel.
func recordsOfKind[T any](s *Store, kind graph.Kind, channelID string) ([]T, error) {
	q := `SELECT record FROM nodes WHERE kind = ? AND record IS NOT NULL`
	args := []any{string(kind)}
	if channelID != "" {
		q += ` AND json_extract(record,'$.channel_id') = ?`
		args = append(args, channelID)
	}
	q += ` ORDER BY uri`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []T
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return out, err
		}
		rec, err := unmarshalRecord[T](blob)
		if err != nil {
			continue
		}
		out = append(out, *rec)
	}
	return out, rows.Err()
}

// storeGetPlaylistItems is the contains claims in their playlist's order, each
// one filled in from the video's record where the store has read it.
func (s *Store) storeGetPlaylistItems(playlistID string) ([]Video, error) {
	return s.videosAcross(graph.PlaylistURI(playlistID), graph.Contains)
}

func (s *Store) storeGetRelated(videoID string) ([]Video, error) {
	return s.videosAcross(graph.VideoURI(videoID), graph.RelatedTo)
}

// videosAcross follows one predicate and returns the videos on the far end.
//
// A node the store has read comes back as its record. A node it has only heard
// of comes back as an id and the title the claim's note carried, which is the
// whole reason the note is a column: a related shelf is thirty videos nobody
// fetched and a list of thirty bare ids is not something a person can read.
func (s *Store) videosAcross(from graph.URI, p graph.Predicate) ([]Video, error) {
	rows, err := s.db.Query(`
		SELECT c.to_uri, c.note, n.record
		FROM claims c LEFT JOIN nodes n ON n.uri = c.to_uri
		WHERE c.from_uri = ? AND c.predicate = ?
		ORDER BY c.position, c.to_uri`, string(from), string(p))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Video
	seen := map[string]bool{}
	for rows.Next() {
		var uri, note string
		var blob []byte
		if err := rows.Scan(&uri, &note, &blob); err != nil {
			return out, err
		}
		parsed, ok := graph.Parse(graph.URI(uri))
		if !ok || seen[parsed.ID] {
			continue
		}
		seen[parsed.ID] = true
		if len(blob) > 0 {
			if v, err := unmarshalRecord[Video](blob); err == nil {
				out = append(out, *v)
				continue
			}
		}
		out = append(out, Video{VideoID: parsed.ID, Title: note, URL: graph.URI(uri).URL()})
	}
	return out, rows.Err()
}

// storeGetChapters comes out of the video's own record rather than a table of
// its own. A chapter has no id and no address, so it was never a node.
func (s *Store) storeGetChapters(videoID string) ([]Chapter, error) {
	n, err := s.Node(graph.VideoURI(videoID))
	if err != nil || n == nil || !n.Read() {
		return nil, err
	}
	v, err := unmarshalRecord[Video](n.Record)
	if err != nil {
		return nil, err
	}
	return v.Chapters, nil
}

func unmarshalRecord[T any](blob []byte) (*T, error) {
	var rec T
	if err := json.Unmarshal(blob, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

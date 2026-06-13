package youtube

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed persistence layer for all crawled YouTube data.
type Store struct {
	db   *sql.DB
	path string
}

// OpenStore opens (or creates) the SQLite database at path and ensures all
// tables exist.
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS videos (
			video_id              TEXT PRIMARY KEY,
			title                 TEXT,
			description           TEXT,
			channel_id            TEXT,
			channel_name          TEXT,
			duration_seconds      INTEGER,
			duration_text         TEXT,
			view_count            INTEGER,
			comment_count         INTEGER,
			like_count            INTEGER,
			published_text        TEXT,
			published_at          TEXT,
			upload_date           TEXT,
			is_live               INTEGER DEFAULT 0,
			is_short              INTEGER DEFAULT 0,
			category              TEXT,
			tags                  TEXT DEFAULT '[]',
			thumbnail_url         TEXT,
			url                   TEXT,
			embed_url             TEXT,
			transcript            TEXT,
			transcript_language   TEXT,
			available_countries   TEXT DEFAULT '[]',
			is_family_safe        INTEGER DEFAULT 1,
			allow_ratings         INTEGER DEFAULT 1,
			age_restricted        INTEGER DEFAULT 0,
			location_description  TEXT,
			hashtags              TEXT DEFAULT '[]',
			fetched_at            TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			channel_id           TEXT PRIMARY KEY,
			handle               TEXT,
			title                TEXT,
			description          TEXT,
			avatar_url           TEXT,
			banner_url           TEXT,
			subscribers_text     TEXT,
			videos_text          TEXT,
			views_text           TEXT,
			country              TEXT,
			joined_date_text     TEXT,
			uploads_playlist_id  TEXT,
			url                  TEXT,
			subscriber_count     INTEGER,
			video_count          INTEGER,
			view_count           INTEGER,
			keywords             TEXT DEFAULT '[]',
			trailer_video_id     TEXT,
			is_verified          INTEGER DEFAULT 0,
			fetched_at           TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS playlists (
			playlist_id       TEXT PRIMARY KEY,
			title             TEXT,
			description       TEXT,
			channel_id        TEXT,
			channel_name      TEXT,
			video_count       INTEGER,
			view_count_text   TEXT,
			last_updated_text TEXT,
			url               TEXT,
			fetched_at        TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS playlist_videos (
			playlist_id TEXT NOT NULL,
			video_id    TEXT NOT NULL,
			position    INTEGER,
			PRIMARY KEY (playlist_id, video_id)
		)`,
		`CREATE TABLE IF NOT EXISTS related_videos (
			video_id         TEXT NOT NULL,
			related_video_id TEXT NOT NULL,
			position         INTEGER,
			PRIMARY KEY (video_id, related_video_id)
		)`,
		`CREATE TABLE IF NOT EXISTS caption_tracks (
			video_id          TEXT NOT NULL,
			language_code     TEXT NOT NULL,
			name              TEXT,
			base_url          TEXT,
			kind              TEXT,
			is_auto_generated INTEGER DEFAULT 0,
			fetched_at        TEXT,
			PRIMARY KEY (video_id, language_code)
		)`,
		`CREATE TABLE IF NOT EXISTS comments (
			id                   TEXT PRIMARY KEY,
			video_id             TEXT NOT NULL,
			parent_id            TEXT,
			author_channel_id    TEXT,
			author_display_name  TEXT,
			author_profile_image TEXT,
			text_display         TEXT,
			like_count           INTEGER DEFAULT 0,
			reply_count          INTEGER DEFAULT 0,
			is_owner_comment     INTEGER DEFAULT 0,
			published_text       TEXT,
			published_at         TEXT,
			updated_at           TEXT,
			fetched_at           TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS chapters (
			video_id      TEXT NOT NULL,
			title         TEXT,
			start_seconds INTEGER NOT NULL,
			thumbnail_url TEXT,
			position      INTEGER,
			PRIMARY KEY (video_id, start_seconds)
		)`,
		`CREATE TABLE IF NOT EXISTS community_posts (
			post_id        TEXT PRIMARY KEY,
			channel_id     TEXT NOT NULL,
			author_name    TEXT,
			author_avatar  TEXT,
			content_text   TEXT,
			like_count     INTEGER DEFAULT 0,
			reply_count    INTEGER DEFAULT 0,
			vote_count     TEXT,
			published_text TEXT,
			attachments    TEXT DEFAULT '[]',
			fetched_at     TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS video_formats (
			video_id       TEXT NOT NULL,
			itag           INTEGER NOT NULL,
			mime_type      TEXT,
			quality        TEXT,
			quality_label  TEXT,
			width          INTEGER,
			height         INTEGER,
			fps            INTEGER,
			bitrate        INTEGER,
			content_length INTEGER,
			is_adaptive    INTEGER DEFAULT 0,
			audio_quality  TEXT,
			PRIMARY KEY (video_id, itag)
		)`,
		`CREATE TABLE IF NOT EXISTS queue (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			url         TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			status      TEXT DEFAULT 'pending',
			priority    INTEGER DEFAULT 0,
			created_at  TEXT,
			updated_at  TEXT
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_queue_url ON queue(url)`,
		`CREATE INDEX IF NOT EXISTS idx_queue_status_priority ON queue(status, priority DESC, created_at)`,
		`CREATE TABLE IF NOT EXISTS jobs (
			job_id       TEXT PRIMARY KEY,
			name         TEXT,
			type         TEXT,
			status       TEXT,
			started_at   TEXT,
			completed_at TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("init schema: %w\nstatement: %s", err, stmt[:min(len(stmt), 80)])
		}
	}
	return nil
}

// --- Video ---

func (s *Store) UpsertVideo(v Video) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO videos (
		video_id, title, description, channel_id, channel_name,
		duration_seconds, duration_text, view_count, comment_count, like_count,
		published_text, published_at, upload_date, is_live, is_short,
		category, tags, thumbnail_url, url, embed_url, transcript, transcript_language,
		available_countries, is_family_safe, allow_ratings, age_restricted,
		location_description, hashtags, fetched_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.VideoID, storeNullStr(v.Title), storeNullStr(v.Description),
		storeNullStr(v.ChannelID), storeNullStr(v.ChannelName),
		storeNullInt(v.DurationSeconds), storeNullStr(v.DurationText),
		v.ViewCount, v.CommentCount, v.LikeCount,
		storeNullStr(v.PublishedText), storeNullTime(v.PublishedAt),
		storeNullStr(v.UploadDate), storeBool(v.IsLive), storeBool(v.IsShort),
		storeNullStr(v.Category), jsonString(v.Tags),
		storeNullStr(v.ThumbnailURL), storeNullStr(v.URL), storeNullStr(v.EmbedURL),
		storeNullStr(v.Transcript), storeNullStr(v.TranscriptLanguage),
		jsonString(v.AvailableCountries),
		storeBool(v.IsFamilySafe), storeBool(v.AllowRatings), storeBool(v.AgeRestricted),
		storeNullStr(v.LocationDescription), jsonString(v.Hashtags),
		storeTime(v.FetchedAt),
	)
	return err
}

// --- Channel ---

func (s *Store) UpsertChannel(c Channel) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO channels (
		channel_id, handle, title, description, avatar_url, banner_url,
		subscribers_text, videos_text, views_text, country, joined_date_text,
		uploads_playlist_id, url,
		subscriber_count, video_count, view_count, keywords, trailer_video_id, is_verified,
		fetched_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ChannelID, storeNullStr(c.Handle), storeNullStr(c.Title),
		storeNullStr(c.Description), storeNullStr(c.AvatarURL), storeNullStr(c.BannerURL),
		storeNullStr(c.SubscribersText), storeNullStr(c.VideosText), storeNullStr(c.ViewsText),
		storeNullStr(c.Country), storeNullStr(c.JoinedDateText),
		storeNullStr(c.UploadsPlaylistID), storeNullStr(c.URL),
		storeNullInt64(c.SubscriberCount), storeNullInt64(c.VideoCount), storeNullInt64(c.ViewCount),
		jsonString(c.Keywords), storeNullStr(c.TrailerVideoID), storeBool(c.IsVerified),
		storeTime(c.FetchedAt),
	)
	return err
}

// --- Playlist ---

func (s *Store) UpsertPlaylist(p Playlist) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO playlists (
		playlist_id, title, description, channel_id, channel_name,
		video_count, view_count_text, last_updated_text, url, fetched_at
	) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.PlaylistID, storeNullStr(p.Title), storeNullStr(p.Description),
		storeNullStr(p.ChannelID), storeNullStr(p.ChannelName),
		p.VideoCount, storeNullStr(p.ViewCountText), storeNullStr(p.LastUpdatedText),
		storeNullStr(p.URL), storeTime(p.FetchedAt),
	)
	return err
}

// --- PlaylistVideo ---

func (s *Store) UpsertPlaylistVideo(pv PlaylistVideo) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO playlist_videos (playlist_id, video_id, position) VALUES (?,?,?)`,
		pv.PlaylistID, pv.VideoID, pv.Position,
	)
	return err
}

// --- RelatedVideo ---

func (s *Store) UpsertRelatedVideo(rv RelatedVideo) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO related_videos (video_id, related_video_id, position) VALUES (?,?,?)`,
		rv.VideoID, rv.RelatedVideoID, rv.Position,
	)
	return err
}

// --- CaptionTrack ---

func (s *Store) UpsertCaptionTrack(ct CaptionTrack) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO caption_tracks
			(video_id, language_code, name, base_url, kind, is_auto_generated, fetched_at)
			VALUES (?,?,?,?,?,?,?)`,
		ct.VideoID, ct.LanguageCode, storeNullStr(ct.Name), storeNullStr(ct.BaseURL),
		storeNullStr(ct.Kind), storeBool(ct.IsAutoGenerated), storeTime(ct.FetchedAt),
	)
	return err
}

// --- Comment ---

func (s *Store) UpsertComment(c Comment) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO comments (
		id, video_id, parent_id, author_channel_id, author_display_name,
		author_profile_image, text_display, like_count, reply_count,
		is_owner_comment, published_text, published_at, updated_at, fetched_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.VideoID, storeNullStr(c.ParentID), storeNullStr(c.AuthorChannelID),
		storeNullStr(c.AuthorDisplayName), storeNullStr(c.AuthorProfileImage),
		storeNullStr(c.TextDisplay), c.LikeCount, c.ReplyCount,
		storeBool(c.IsOwnerComment), storeNullStr(c.PublishedText),
		storeNullTime(c.PublishedAt), storeNullTime(c.UpdatedAt), storeTime(c.FetchedAt),
	)
	return err
}

// --- Chapter ---

func (s *Store) UpsertChapter(ch Chapter) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO chapters (video_id, title, start_seconds, thumbnail_url, position)
			VALUES (?,?,?,?,?)`,
		ch.VideoID, storeNullStr(ch.Title), ch.StartSeconds,
		storeNullStr(ch.ThumbnailURL), ch.Position,
	)
	return err
}

// --- CommunityPost ---

func (s *Store) UpsertCommunityPost(p CommunityPost) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO community_posts (
		post_id, channel_id, author_name, author_avatar, content_text,
		like_count, reply_count, vote_count, published_text, attachments, fetched_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.PostID, p.ChannelID, storeNullStr(p.AuthorName), storeNullStr(p.AuthorAvatar),
		storeNullStr(p.ContentText), p.LikeCount, p.ReplyCount,
		storeNullStr(p.VoteCount), storeNullStr(p.PublishedText), p.Attachments,
		storeTime(p.FetchedAt),
	)
	return err
}

// --- VideoFormat ---

func (s *Store) UpsertVideoFormat(f VideoFormat) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO video_formats (
		video_id, itag, mime_type, quality, quality_label, width, height,
		fps, bitrate, content_length, is_adaptive, audio_quality
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.VideoID, f.ITag, storeNullStr(f.MimeType), storeNullStr(f.Quality),
		storeNullStr(f.QualityLabel), storeNullInt(f.Width), storeNullInt(f.Height),
		storeNullInt(f.FPS), f.Bitrate, f.ContentLength, storeBool(f.IsAdaptive),
		storeNullStr(f.AudioQuality),
	)
	return err
}

// --- Queue ---

// Enqueue adds a URL to the crawl queue. Duplicate URLs are silently ignored.
func (s *Store) Enqueue(url, entity string, priority int) error {
	now := storeTime(time.Now())
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO queue (url, entity_type, priority, status, created_at, updated_at)
			VALUES (?,?,?,'pending',?,?)`,
		url, entity, priority, now, now,
	)
	return err
}

// NextPending atomically pops the highest-priority pending item and marks it
// in_progress. Returns nil, nil if the queue is empty.
func (s *Store) NextPending() (*QueueItem, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRow(
		`SELECT id, url, entity_type, status, priority
			FROM queue
			WHERE status = 'pending'
			ORDER BY priority DESC, created_at ASC
			LIMIT 1`)
	var it QueueItem
	if err := row.Scan(&it.ID, &it.URL, &it.EntityType, &it.Status, &it.Priority); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	now := storeTime(time.Now())
	if _, err := tx.Exec(
		`UPDATE queue SET status='in_progress', updated_at=? WHERE id=?`, now, it.ID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	it.Status = "in_progress"
	return &it, nil
}

// MarkStatus updates the status of a queue item by its row ID.
func (s *Store) MarkStatus(id int64, status string) error {
	_, err := s.db.Exec(
		`UPDATE queue SET status=?, updated_at=? WHERE id=?`,
		status, storeTime(time.Now()), id,
	)
	return err
}

// ListQueue returns up to limit items with the given status.
func (s *Store) ListQueue(status string, limit int) ([]QueueItem, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT id, url, entity_type, status, priority
			FROM queue WHERE status=? ORDER BY priority DESC, created_at ASC LIMIT ?`,
		status, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []QueueItem
	for rows.Next() {
		var it QueueItem
		if err := rows.Scan(&it.ID, &it.URL, &it.EntityType, &it.Status, &it.Priority); err != nil {
			return out, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// --- Jobs ---

// RecordJob inserts or replaces a job record.
func (s *Store) RecordJob(j JobRecord) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO jobs (job_id, name, type, status, started_at, completed_at)
			VALUES (?,?,?,?,?,?)`,
		j.JobID, storeNullStr(j.Name), storeNullStr(j.Type), storeNullStr(j.Status),
		storeNullTime(j.StartedAt), storeNullTime(j.CompletedAt),
	)
	return err
}

// ListJobs returns up to limit jobs ordered by started_at DESC.
func (s *Store) ListJobs(limit int) ([]JobRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT job_id, COALESCE(name,''), COALESCE(type,''), COALESCE(status,''),
			COALESCE(started_at,''), COALESCE(completed_at,'')
			FROM jobs ORDER BY started_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []JobRecord
	for rows.Next() {
		var j JobRecord
		var startedStr, completedStr string
		if err := rows.Scan(&j.JobID, &j.Name, &j.Type, &j.Status, &startedStr, &completedStr); err != nil {
			return out, err
		}
		j.StartedAt, _ = parseStoreTime(startedStr)
		j.CompletedAt, _ = parseStoreTime(completedStr)
		out = append(out, j)
	}
	return out, rows.Err()
}

// --- Queries ---

// Stats returns row counts for all major tables.
func (s *Store) Stats() (map[string]int64, error) {
	tables := []string{
		"videos", "channels", "playlists", "playlist_videos", "related_videos",
		"caption_tracks", "comments", "chapters", "community_posts", "video_formats",
		"queue", "jobs",
	}
	out := make(map[string]int64, len(tables))
	for _, t := range tables {
		var n int64
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM ` + t).Scan(&n)
		out[t] = n
	}
	return out, nil
}

// SearchVideos performs a LIKE search on video title and description.
func (s *Store) SearchVideos(q string, limit int) ([]Video, error) {
	if limit <= 0 {
		limit = 20
	}
	pat := "%" + strings.ToLower(q) + "%"
	rows, err := s.db.Query(`
		SELECT video_id, COALESCE(title,''), COALESCE(description,''),
		       COALESCE(channel_id,''), COALESCE(channel_name,''),
		       COALESCE(duration_seconds,0), COALESCE(duration_text,''),
		       COALESCE(view_count,0), COALESCE(published_text,''), COALESCE(url,'')
		FROM videos
		WHERE lower(title) LIKE ? OR lower(description) LIKE ?
		ORDER BY
			CASE WHEN lower(title) LIKE ? THEN 0 ELSE 1 END,
			view_count DESC
		LIMIT ?`, pat, pat, pat, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(
			&v.VideoID, &v.Title, &v.Description,
			&v.ChannelID, &v.ChannelName,
			&v.DurationSeconds, &v.DurationText,
			&v.ViewCount, &v.PublishedText, &v.URL,
		); err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SearchChannels performs a LIKE search on channel title, description, and handle.
func (s *Store) SearchChannels(q string, limit int) ([]Channel, error) {
	if limit <= 0 {
		limit = 20
	}
	pat := "%" + strings.ToLower(q) + "%"
	rows, err := s.db.Query(`
		SELECT channel_id, COALESCE(handle,''), COALESCE(title,''),
		       COALESCE(description,''), COALESCE(subscribers_text,''), COALESCE(url,'')
		FROM channels
		WHERE lower(title) LIKE ? OR lower(description) LIKE ? OR lower(handle) LIKE ?
		ORDER BY
			CASE WHEN lower(title) LIKE ? THEN 0 ELSE 1 END,
			title ASC
		LIMIT ?`, pat, pat, pat, pat, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(
			&c.ChannelID, &c.Handle, &c.Title, &c.Description, &c.SubscribersText, &c.URL,
		); err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Query executes a raw read-only SQL statement and returns column names + rows.
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

// Path returns the filesystem path of the database file.
func (s *Store) Path() string { return s.path }

// Vacuum runs SQLite VACUUM to reclaim space.
func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}

// Reset drops and recreates all tables.
func (s *Store) Reset() error {
	tables := []string{
		"videos", "channels", "playlists", "playlist_videos", "related_videos",
		"caption_tracks", "comments", "chapters", "community_posts", "video_formats",
		"queue", "jobs",
	}
	for _, t := range tables {
		if _, err := s.db.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return fmt.Errorf("drop %s: %w", t, err)
		}
	}
	return s.initSchema()
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// --- Store-internal read helpers used by export.go ---

func (s *Store) storeGetChannel(idOrHandle string) (*Channel, error) {
	h := strings.TrimPrefix(idOrHandle, "@")
	row := s.db.QueryRow(`
		SELECT channel_id, COALESCE(handle,''), COALESCE(title,''),
		       COALESCE(description,''), COALESCE(avatar_url,''), COALESCE(banner_url,''),
		       COALESCE(subscribers_text,''), COALESCE(videos_text,''),
		       COALESCE(views_text,''), COALESCE(country,''),
		       COALESCE(joined_date_text,''), COALESCE(uploads_playlist_id,''),
		       COALESCE(url,'')
		FROM channels
		WHERE channel_id=? OR handle=? OR handle=?
		   OR handle LIKE '%/@'||? OR handle LIKE '%/'||?
		   OR lower(title)=lower(?)
		LIMIT 1`,
		idOrHandle, h, "@"+h, h, h, h)
	var c Channel
	if err := row.Scan(
		&c.ChannelID, &c.Handle, &c.Title,
		&c.Description, &c.AvatarURL, &c.BannerURL,
		&c.SubscribersText, &c.VideosText,
		&c.ViewsText, &c.Country,
		&c.JoinedDateText, &c.UploadsPlaylistID,
		&c.URL,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) storeGetAllChannels() ([]Channel, error) {
	rows, err := s.db.Query(`
		SELECT channel_id, COALESCE(handle,''), COALESCE(title,''),
		       COALESCE(description,''), COALESCE(avatar_url,''), COALESCE(banner_url,''),
		       COALESCE(subscribers_text,''), COALESCE(videos_text,''),
		       COALESCE(views_text,''), COALESCE(country,''),
		       COALESCE(joined_date_text,''), COALESCE(uploads_playlist_id,''),
		       COALESCE(url,'')
		FROM channels ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(
			&c.ChannelID, &c.Handle, &c.Title,
			&c.Description, &c.AvatarURL, &c.BannerURL,
			&c.SubscribersText, &c.VideosText,
			&c.ViewsText, &c.Country,
			&c.JoinedDateText, &c.UploadsPlaylistID,
			&c.URL,
		); err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) storeGetVideosByChannel(channelID, channelName string) ([]Video, error) {
	rows, err := s.db.Query(`
		SELECT video_id, COALESCE(title,''), COALESCE(description,''),
		       COALESCE(channel_id,''), COALESCE(channel_name,''),
		       COALESCE(duration_seconds,0), COALESCE(duration_text,''),
		       COALESCE(view_count,0), COALESCE(comment_count,0), COALESCE(like_count,0),
		       COALESCE(published_text,''), COALESCE(upload_date,''),
		       COALESCE(is_live,0), COALESCE(is_short,0),
		       COALESCE(tags,'[]'), COALESCE(thumbnail_url,''), COALESCE(url,''),
		       COALESCE(transcript,''), COALESCE(transcript_language,''),
		       COALESCE(published_at,'')
		FROM videos WHERE channel_id=? OR channel_name=?
		ORDER BY fetched_at DESC`,
		channelID, channelName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Video
	for rows.Next() {
		var v Video
		var tags, pubAtStr string
		if err := rows.Scan(
			&v.VideoID, &v.Title, &v.Description,
			&v.ChannelID, &v.ChannelName,
			&v.DurationSeconds, &v.DurationText,
			&v.ViewCount, &v.CommentCount, &v.LikeCount,
			&v.PublishedText, &v.UploadDate,
			&v.IsLive, &v.IsShort,
			&tags, &v.ThumbnailURL, &v.URL,
			&v.Transcript, &v.TranscriptLanguage,
			&pubAtStr,
		); err != nil {
			return out, err
		}
		if tags != "" && tags != "[]" {
			_ = json.Unmarshal([]byte(tags), &v.Tags)
		}
		v.PublishedAt, _ = parseStoreTime(pubAtStr)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) storeGetPlaylistsByChannel(channelID, channelName string) ([]Playlist, error) {
	rows, err := s.db.Query(`
		SELECT playlist_id, COALESCE(title,''), COALESCE(description,''),
		       COALESCE(channel_id,''), COALESCE(channel_name,''),
		       COALESCE(video_count,0), COALESCE(view_count_text,''),
		       COALESCE(last_updated_text,''), COALESCE(url,'')
		FROM playlists WHERE channel_id=? OR channel_name=?
		ORDER BY title`,
		channelID, channelName)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Playlist
	for rows.Next() {
		var p Playlist
		if err := rows.Scan(
			&p.PlaylistID, &p.Title, &p.Description,
			&p.ChannelID, &p.ChannelName,
			&p.VideoCount, &p.ViewCountText,
			&p.LastUpdatedText, &p.URL,
		); err != nil {
			return out, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) storeGetPlaylistItems(playlistID string) ([]Video, error) {
	rows, err := s.db.Query(`
		SELECT pv.video_id,
		       COALESCE(v.title,''), COALESCE(v.channel_name,''),
		       COALESCE(v.duration_text,''), COALESCE(v.duration_seconds,0),
		       COALESCE(v.view_count,0), COALESCE(v.thumbnail_url,''),
		       COALESCE(v.description,''), COALESCE(v.published_text,''),
		       COALESCE(v.upload_date,'')
		FROM playlist_videos pv
		LEFT JOIN videos v ON v.video_id=pv.video_id
		WHERE pv.playlist_id=?
		ORDER BY pv.position`, playlistID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(
			&v.VideoID, &v.Title, &v.ChannelName,
			&v.DurationText, &v.DurationSeconds,
			&v.ViewCount, &v.ThumbnailURL,
			&v.Description, &v.PublishedText, &v.UploadDate,
		); err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) storeGetRelated(videoID string) ([]Video, error) {
	rows, err := s.db.Query(`
		SELECT rv.related_video_id,
		       COALESCE(v.title,''), COALESCE(v.channel_name,''),
		       COALESCE(v.duration_text,''), COALESCE(v.view_count,0)
		FROM related_videos rv
		LEFT JOIN videos v ON v.video_id=rv.related_video_id
		WHERE rv.video_id=?
		ORDER BY rv.position`, videoID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Video
	for rows.Next() {
		var v Video
		if err := rows.Scan(
			&v.VideoID, &v.Title, &v.ChannelName, &v.DurationText, &v.ViewCount,
		); err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) storeGetChapters(videoID string) ([]Chapter, error) {
	rows, err := s.db.Query(`
		SELECT video_id, COALESCE(title,''), start_seconds,
		       COALESCE(thumbnail_url,''), COALESCE(position,0)
		FROM chapters WHERE video_id=? ORDER BY position`, videoID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Chapter
	for rows.Next() {
		var c Chapter
		if err := rows.Scan(&c.VideoID, &c.Title, &c.StartSeconds, &c.ThumbnailURL, &c.Position); err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- Low-level helpers (store-private, prefixed to avoid collision) ---

func storeNullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func storeNullInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func storeNullInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func storeBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

func storeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func storeNullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func parseStoreTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// jsonString encodes a string slice as a compact JSON array.
func jsonString(v []string) string {
	if len(v) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

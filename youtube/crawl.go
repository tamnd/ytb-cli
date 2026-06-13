package youtube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

// CrawlOptions controls how the Crawl loop runs.
type CrawlOptions struct {
	// Workers is the number of concurrent queue-pop goroutines (1 = sequential).
	Workers int
	// Entity restricts crawling to a specific entity type (empty = all).
	Entity string
	// MaxPerItem caps the number of items fetched per queue entry (0 = unlimited).
	MaxPerItem int
}

// Crawl pops items off the store queue and fetches each one using c, storing
// results into store. It runs until the queue is empty or ctx is cancelled.
//
// If opt.Workers > 1 a concurrent errgroup is used; any per-item error is
// logged via logf and does not abort the overall crawl.
func Crawl(ctx context.Context, c *Client, store *Store, opt CrawlOptions, logf func(string)) error {
	if logf == nil {
		logf = func(string) {}
	}
	workers := opt.Workers
	if workers <= 0 {
		workers = 1
	}

	// Sequential path avoids the overhead of errgroup when workers == 1.
	if workers == 1 {
		return crawlSequential(ctx, c, store, opt, logf)
	}
	return crawlConcurrent(ctx, c, store, opt, logf, workers)
}

func crawlSequential(ctx context.Context, c *Client, store *Store, opt CrawlOptions, logf func(string)) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		item, err := store.NextPending()
		if err != nil {
			return fmt.Errorf("queue pop: %w", err)
		}
		if item == nil {
			return nil
		}
		if opt.Entity != "" && item.EntityType != opt.Entity {
			// Skip mismatched entity types but still mark them done so we don't loop forever.
			_ = store.MarkStatus(item.ID, "skipped")
			continue
		}
		crawlItem(ctx, c, store, *item, opt, logf)
	}
}

func crawlConcurrent(ctx context.Context, c *Client, store *Store, opt CrawlOptions, logf func(string), workers int) error {
	eg, egCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, workers)
	for egCtx.Err() == nil {
		item, err := store.NextPending()
		if err != nil {
			break
		}
		if item == nil {
			break
		}
		if opt.Entity != "" && item.EntityType != opt.Entity {
			_ = store.MarkStatus(item.ID, "skipped")
			continue
		}
		it := *item
		sem <- struct{}{}
		eg.Go(func() error {
			defer func() { <-sem }()
			crawlItem(egCtx, c, store, it, opt, logf)
			return nil
		})
	}
	return eg.Wait()
}

// crawlItem fetches one queue item and upserts everything into the store.
// Per-item errors are logged via logf and the item is marked failed, not returned.
func crawlItem(ctx context.Context, c *Client, store *Store, item QueueItem, opt CrawlOptions, logf func(string)) {
	logf(fmt.Sprintf("crawl %s %s", item.EntityType, item.URL))
	start := time.Now()
	job := JobRecord{
		JobID:     fmt.Sprintf("%s-%d", item.EntityType, item.ID),
		Name:      item.URL,
		Type:      item.EntityType,
		Status:    "running",
		StartedAt: start,
	}
	_ = store.RecordJob(job)

	var crawlErr error
	switch item.EntityType {
	case EntityVideo:
		crawlErr = crawlVideo(ctx, c, store, item.URL, opt)
	case EntityChannel:
		crawlErr = crawlChannel(ctx, c, store, item.URL, opt)
	case EntityPlaylist:
		crawlErr = crawlPlaylist(ctx, c, store, item.URL, opt)
	case EntitySearch:
		crawlErr = crawlSearch(ctx, c, store, item.URL, opt)
	case EntityHashtag:
		crawlErr = crawlHashtag(ctx, c, store, item.URL, opt)
	case EntityComments:
		crawlErr = crawlComments(ctx, c, store, item.URL, opt)
	case EntityCommunity:
		crawlErr = crawlCommunity(ctx, c, store, item.URL, opt)
	default:
		crawlErr = fmt.Errorf("unknown entity type %q", item.EntityType)
	}

	job.CompletedAt = time.Now()
	if crawlErr != nil {
		logf(fmt.Sprintf("ERROR %s %s: %v", item.EntityType, item.URL, crawlErr))
		job.Status = "failed"
		_ = store.MarkStatus(item.ID, "failed")
	} else {
		job.Status = "done"
		_ = store.MarkStatus(item.ID, "done")
	}
	_ = store.RecordJob(job)
}

// --- per-entity crawlers ---

func crawlVideo(ctx context.Context, c *Client, store *Store, url string, opt CrawlOptions) error {
	result, err := c.FetchVideo(ctx, url, VideoOptions{Player: true, Next: true})
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	if err := store.UpsertVideo(result.Video); err != nil {
		return fmt.Errorf("upsert video: %w", err)
	}
	for _, f := range result.Formats {
		_ = store.UpsertVideoFormat(f)
	}
	for _, ct := range result.Captions {
		_ = store.UpsertCaptionTrack(ct)
	}
	for _, ch := range result.Chapters {
		_ = store.UpsertChapter(ch)
	}
	for _, rv := range result.Related {
		_ = store.UpsertRelatedVideo(rv)
	}
	return nil
}

func crawlChannel(ctx context.Context, c *Client, store *Store, url string, opt CrawlOptions) error {
	ch, err := c.FetchChannel(ctx, url)
	if err != nil {
		return err
	}
	if ch != nil {
		if err := store.UpsertChannel(*ch); err != nil {
			return fmt.Errorf("upsert channel: %w", err)
		}
	}
	po := PageOptions{Max: opt.MaxPerItem}
	streamErr := c.StreamChannelTab(ctx, url, "videos", po, func(v Video) error {
		if err := store.UpsertVideo(v); err != nil {
			return fmt.Errorf("upsert video in channel stream: %w", err)
		}
		return nil
	})
	if streamErr != nil && !errors.Is(streamErr, ErrStop) {
		return streamErr
	}
	return nil
}

func crawlPlaylist(ctx context.Context, c *Client, store *Store, url string, opt CrawlOptions) error {
	pl, err := c.FetchPlaylist(ctx, url)
	if err != nil {
		return err
	}
	if pl != nil {
		if err := store.UpsertPlaylist(*pl); err != nil {
			return fmt.Errorf("upsert playlist: %w", err)
		}
	}
	po := PageOptions{Max: opt.MaxPerItem}
	streamErr := c.StreamPlaylistItems(ctx, url, po, func(pv PlaylistVideo, v Video) error {
		_ = store.UpsertPlaylistVideo(pv)
		if v.VideoID != "" {
			_ = store.UpsertVideo(v)
		}
		return nil
	})
	if streamErr != nil && !errors.Is(streamErr, ErrStop) {
		return streamErr
	}
	return nil
}

func crawlSearch(ctx context.Context, c *Client, store *Store, query string, opt CrawlOptions) error {
	po := PageOptions{Max: opt.MaxPerItem}
	streamErr := c.Search(ctx, query, SearchFilters{}, po, func(item any) error {
		switch v := item.(type) {
		case Video:
			_ = store.UpsertVideo(v)
		case Channel:
			_ = store.UpsertChannel(v)
		case Playlist:
			_ = store.UpsertPlaylist(v)
		}
		return nil
	})
	if streamErr != nil && !errors.Is(streamErr, ErrStop) {
		return streamErr
	}
	return nil
}

func crawlHashtag(ctx context.Context, c *Client, store *Store, tag string, opt CrawlOptions) error {
	// Strip leading "#" for the API; some queue entries may include it.
	tag = strings.TrimPrefix(tag, "#")
	po := PageOptions{Max: opt.MaxPerItem}
	streamErr := c.StreamHashtag(ctx, tag, po, func(v Video) error {
		return store.UpsertVideo(v)
	})
	if streamErr != nil && !errors.Is(streamErr, ErrStop) {
		return streamErr
	}
	return nil
}

func crawlComments(ctx context.Context, c *Client, store *Store, url string, opt CrawlOptions) error {
	co := CommentOptions{Max: opt.MaxPerItem, Replies: false, Sort: "top"}
	streamErr := c.StreamComments(ctx, url, co, func(cm Comment) error {
		return store.UpsertComment(cm)
	})
	if streamErr != nil && !errors.Is(streamErr, ErrStop) {
		return streamErr
	}
	return nil
}

func crawlCommunity(ctx context.Context, c *Client, store *Store, channel string, opt CrawlOptions) error {
	po := PageOptions{Max: opt.MaxPerItem}
	streamErr := c.StreamCommunity(ctx, channel, po, func(p CommunityPost) error {
		return store.UpsertCommunityPost(p)
	})
	if streamErr != nil && !errors.Is(streamErr, ErrStop) {
		return streamErr
	}
	return nil
}

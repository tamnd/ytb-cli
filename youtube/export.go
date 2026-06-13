package youtube

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Export writes Markdown pages for all channels (channel=="") or a single
// channel identified by @handle or UC-style ID.
//
// Output layout mirrors the reference exporter:
//
//	outDir/<handle-or-id>/README.md          channel index
//	                      videos/README.md    all-videos index
//	                      videos/YYYY/MM/DD/<slug>-<id>.md
//	                      playlists/README.md
//	                      playlists/<slug>.md
func Export(store *Store, channel, outDir string) error {
	if channel == "" {
		channels, err := store.storeGetAllChannels()
		if err != nil {
			return fmt.Errorf("list channels: %w", err)
		}
		for _, ch := range channels {
			if err := exportChannel(store, ch, outDir); err != nil {
				return err
			}
		}
		return nil
	}
	ch, err := store.storeGetChannel(channel)
	if err != nil {
		return fmt.Errorf("channel %q not found: %w", channel, err)
	}
	return exportChannel(store, *ch, outDir)
}

func exportChannel(store *Store, ch Channel, outDir string) error {
	videos, err := store.storeGetVideosByChannel(ch.ChannelID, ch.Title)
	if err != nil {
		return fmt.Errorf("get videos for %s: %w", ch.ChannelID, err)
	}
	playlists, err := store.storeGetPlaylistsByChannel(ch.ChannelID, ch.Title)
	if err != nil {
		return fmt.Errorf("get playlists for %s: %w", ch.ChannelID, err)
	}

	// Separate shorts from regular videos.
	var regular, shorts []Video
	for _, v := range videos {
		if v.IsShort {
			shorts = append(shorts, v)
		} else {
			regular = append(regular, v)
		}
	}

	fileMap := make(map[string]string, len(regular))
	for _, v := range regular {
		fileMap[v.VideoID] = exportVideoRelPath(v)
	}
	shortsMap := make(map[string]string, len(shorts))
	for _, v := range shorts {
		shortsMap[v.VideoID] = exportVideoRelPath(v)
	}
	plMap := make(map[string]string, len(playlists))
	for _, p := range playlists {
		plMap[p.PlaylistID] = exportPlaylistSlug(p)
	}

	dir := filepath.Join(outDir, exportChannelDir(ch))
	subs := []string{"videos", "playlists"}
	if len(shorts) > 0 {
		subs = append(subs, "shorts")
	}
	for _, sub := range subs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}

	// Create date-based subdirs for video pages.
	hasTranscripts := false
	created := map[string]bool{}
	for _, rel := range fileMap {
		d := filepath.Dir(rel)
		if !created[d] {
			created[d] = true
			_ = os.MkdirAll(filepath.Join(dir, "videos", d), 0o755)
		}
		// Check if any video has a transcript (brute-force scan once).
	}
	for _, v := range regular {
		if v.Transcript != "" {
			hasTranscripts = true
			break
		}
	}
	if hasTranscripts {
		for _, rel := range fileMap {
			d := filepath.Dir(rel)
			_ = os.MkdirAll(filepath.Join(dir, "transcripts", d), 0o755)
		}
	}
	for _, rel := range shortsMap {
		d := filepath.Dir(rel)
		if !created["s:"+d] {
			created["s:"+d] = true
			_ = os.MkdirAll(filepath.Join(dir, "shorts", d), 0o755)
		}
	}

	// Combined map for playlist cross-links.
	allMap := make(map[string]string, len(fileMap)+len(shortsMap))
	for k, v := range fileMap {
		allMap[k] = "videos/" + v
	}
	for k, v := range shortsMap {
		allMap[k] = "shorts/" + v
	}

	// Pre-fetch playlist items.
	plItems := make(map[string][]Video, len(playlists))
	for _, p := range playlists {
		items, _ := store.storeGetPlaylistItems(p.PlaylistID)
		plItems[p.PlaylistID] = items
	}

	if err := exportWriteChannelIndex(ch, regular, shorts, playlists, plItems, fileMap, shortsMap, plMap, dir); err != nil {
		return err
	}
	if err := exportWriteVideosIndex(regular, fileMap, "videos", dir); err != nil {
		return err
	}
	for _, v := range regular {
		related, _ := store.storeGetRelated(v.VideoID)
		chapters, _ := store.storeGetChapters(v.VideoID)
		if err := exportWriteVideoPage(v, related, chapters, fileMap, "videos", dir); err != nil {
			return err
		}
		if v.Transcript != "" {
			if err := exportWriteTranscript(v, fileMap, dir); err != nil {
				return err
			}
		}
	}
	if len(shorts) > 0 {
		if err := exportWriteVideosIndex(shorts, shortsMap, "shorts", dir); err != nil {
			return err
		}
		for _, v := range shorts {
			related, _ := store.storeGetRelated(v.VideoID)
			chapters, _ := store.storeGetChapters(v.VideoID)
			if err := exportWriteVideoPage(v, related, chapters, shortsMap, "shorts", dir); err != nil {
				return err
			}
		}
	}
	if len(playlists) > 0 {
		if err := exportWritePlaylistsIndex(playlists, plItems, plMap, dir); err != nil {
			return err
		}
		for _, p := range playlists {
			if err := exportWritePlaylistPage(p, plItems[p.PlaylistID], allMap, plMap, dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- channel index ---

func exportWriteChannelIndex(ch Channel, videos, shorts []Video, playlists []Playlist, plItems map[string][]Video, fileMap, shortsMap, plMap map[string]string, dir string) error {
	f, err := os.Create(filepath.Join(dir, "README.md"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	all := append(append([]Video{}, videos...), shorts...)
	var totalViews, totalDur int64
	var earliest, latest string
	yearCounts := map[string]int{}
	for _, v := range all {
		totalViews += v.ViewCount
		totalDur += int64(v.DurationSeconds)
		d := exportVideoDate(v)
		if d != "0000-00-00" {
			y := d[:4]
			yearCounts[y]++
			if earliest == "" || d < earliest {
				earliest = d
			}
			if d > latest {
				latest = d
			}
		}
	}

	byViews := make([]Video, len(videos))
	copy(byViews, videos)
	sort.Slice(byViews, func(i, j int) bool { return byViews[i].ViewCount > byViews[j].ViewCount })

	byDate := make([]Video, len(videos))
	copy(byDate, videos)
	sort.Slice(byDate, func(i, j int) bool { return exportVideoDate(byDate[i]) > exportVideoDate(byDate[j]) })

	_, _ = fmt.Fprintf(f, "> [!NOTE]\n")
	_, _ = fmt.Fprintf(f, "> **Copyright notice.** All videos, thumbnails, and descriptions linked from this repository belong to\n")
	_, _ = fmt.Fprintf(f, "> [%s](%s) and their respective creators.\n", ch.Title, ch.URL)
	_, _ = fmt.Fprintf(f, "> This repository is an unofficial index of publicly available content. It contains only\n")
	_, _ = fmt.Fprintf(f, "> metadata, links, and short excerpts used for reference. No video files are hosted here.\n")
	_, _ = fmt.Fprintf(f, "> If you are the content owner and want this taken down, please open an issue.\n\n")

	_, _ = fmt.Fprintf(f, "<!--\nchannel_id: %s\n", ch.ChannelID)
	if ch.Handle != "" {
		_, _ = fmt.Fprintf(f, "handle: %s\n", ch.Handle)
	}
	_, _ = fmt.Fprintf(f, "title: %s\nurl: %s\n-->\n\n", ch.Title, ch.URL)

	_, _ = fmt.Fprintf(f, "# %s\n\n", ch.Title)
	if ch.AvatarURL != "" {
		_, _ = fmt.Fprintf(f, "<img src=\"%s\" alt=\"%s\" width=\"120\">\n\n", ch.AvatarURL, exportEscText(ch.Title))
	}
	if ch.Description != "" {
		_, _ = fmt.Fprintf(f, "> %s\n\n", strings.ReplaceAll(strings.TrimSpace(ch.Description), "\n", "\n> "))
	}

	_, _ = fmt.Fprintf(f, "## Channel at a Glance\n\n")
	_, _ = fmt.Fprintf(f, "| | |\n|---|---|\n")
	exportStatRow(f, "Subscribers", ch.SubscribersText)
	exportStatRow(f, "Videos", fmt.Sprintf("%d", len(videos)))
	if len(shorts) > 0 {
		exportStatRow(f, "Shorts", fmt.Sprintf("[%d](shorts/README.md)", len(shorts)))
	}
	if len(playlists) > 0 {
		exportStatRow(f, "Playlists", fmt.Sprintf("[%d](playlists/README.md)", len(playlists)))
	}
	if totalViews > 0 && len(videos) > 0 {
		exportStatRow(f, "Total Views", exportFmtCount(totalViews))
		exportStatRow(f, "Avg Views/Video", exportFmtCount(totalViews/int64(len(videos))))
	}
	if totalDur > 0 {
		exportStatRow(f, "Total Watch Time", exportFmtDuration(totalDur))
		if len(videos) > 0 {
			exportStatRow(f, "Avg Duration", exportFmtDurationShort(totalDur/int64(len(videos))))
		}
	}
	if earliest != "" && latest != "" {
		exportStatRow(f, "Content Span", fmt.Sprintf("%s to %s (%d years)", earliest[:4], latest[:4], len(yearCounts)))
	}
	exportStatRow(f, "Country", ch.Country)
	exportStatRow(f, "Joined", ch.JoinedDateText)
	_, _ = fmt.Fprintf(f, "| **YouTube** | [%s](%s) |\n\n", ch.URL, ch.URL)

	if len(byViews) > 0 {
		lim := 10
		if len(byViews) < lim {
			lim = len(byViews)
		}
		_, _ = fmt.Fprintf(f, "## Most Popular\n\n")
		_, _ = fmt.Fprintf(f, "| # | Title | Views | Duration |\n|---|-------|-------|----------|\n")
		for i, v := range byViews[:lim] {
			rel := fileMap[v.VideoID]
			_, _ = fmt.Fprintf(f, "| %d | [%s](videos/%s.md) | %s | %s |\n",
				i+1, exportEscTbl(v.Title), rel, exportFmtCount(v.ViewCount), v.DurationText)
		}
		_, _ = fmt.Fprintln(f)
	}

	if len(byDate) > 0 {
		lim := 10
		if len(byDate) < lim {
			lim = len(byDate)
		}
		_, _ = fmt.Fprintf(f, "## Recent Uploads\n\n")
		_, _ = fmt.Fprintf(f, "| # | Title | Published | Views |\n|---|-------|-----------|-------|\n")
		for i, v := range byDate[:lim] {
			rel := fileMap[v.VideoID]
			_, _ = fmt.Fprintf(f, "| %d | [%s](videos/%s.md) | %s | %s |\n",
				i+1, exportEscTbl(v.Title), rel, exportVideoDate(v), exportFmtCount(v.ViewCount))
		}
		_, _ = fmt.Fprintln(f)
	}

	if len(yearCounts) > 0 {
		years := make([]string, 0, len(yearCounts))
		maxCount := 0
		for y, c := range yearCounts {
			years = append(years, y)
			if c > maxCount {
				maxCount = c
			}
		}
		sort.Strings(years)
		_, _ = fmt.Fprintf(f, "## Videos by Year\n\n```\n")
		for _, y := range years {
			c := yearCounts[y]
			barLen := c * 40 / maxCount
			if barLen == 0 && c > 0 {
				barLen = 1
			}
			_, _ = fmt.Fprintf(f, "%s  %s %d\n", y, strings.Repeat("█", barLen), c)
		}
		_, _ = fmt.Fprintf(f, "```\n\n")
	}

	_, _ = fmt.Fprintf(f, "## [All Videos](videos/README.md) (%d)\n\n", len(videos))
	lim := 20
	if len(videos) < lim {
		lim = len(videos)
	}
	if lim > 0 {
		_, _ = fmt.Fprintf(f, "| Title | Duration | Views |\n|-------|----------|-------|\n")
		for _, v := range videos[:lim] {
			rel := fileMap[v.VideoID]
			_, _ = fmt.Fprintf(f, "| [%s](videos/%s.md) | %s | %s |\n",
				exportEscTbl(v.Title), rel, v.DurationText, exportFmtCount(v.ViewCount))
		}
		if len(videos) > 20 {
			_, _ = fmt.Fprintf(f, "\n*...and %d more -> [all videos](videos/README.md)*\n", len(videos)-20)
		}
		_, _ = fmt.Fprintln(f)
	}

	if len(playlists) > 0 {
		_, _ = fmt.Fprintf(f, "## [Playlists](playlists/README.md) (%d)\n\n", len(playlists))
		_, _ = fmt.Fprintf(f, "| Title | Videos | Views | Duration |\n|-------|--------|-------|----------|\n")
		for _, p := range playlists {
			slug := plMap[p.PlaylistID]
			items := plItems[p.PlaylistID]
			var views, dur int64
			for _, v := range items {
				views += v.ViewCount
				dur += int64(v.DurationSeconds)
			}
			_, _ = fmt.Fprintf(f, "| [%s](playlists/%s.md) | %d | %s | %s |\n",
				exportEscTbl(p.Title), slug, p.VideoCount, exportFmtCount(views), exportFmtDurationShort(dur))
		}
		_, _ = fmt.Fprintln(f)
	}

	if len(shorts) > 0 {
		_, _ = fmt.Fprintf(f, "## [Shorts](shorts/README.md) (%d)\n\n", len(shorts))
		srtd := make([]Video, len(shorts))
		copy(srtd, shorts)
		sort.Slice(srtd, func(i, j int) bool { return srtd[i].ViewCount > srtd[j].ViewCount })
		lim := 10
		if len(srtd) < lim {
			lim = len(srtd)
		}
		_, _ = fmt.Fprintf(f, "| Title | Views |\n|-------|-------|\n")
		for _, v := range srtd[:lim] {
			rel := shortsMap[v.VideoID]
			_, _ = fmt.Fprintf(f, "| [%s](shorts/%s.md) | %s |\n",
				exportEscTbl(v.Title), rel, exportFmtCount(v.ViewCount))
		}
		if len(shorts) > lim {
			_, _ = fmt.Fprintf(f, "\n*...and %d more -> [all shorts](shorts/README.md)*\n", len(shorts)-lim)
		}
		_, _ = fmt.Fprintln(f)
	}

	_, _ = fmt.Fprintf(f, "---\n\n*This index was generated automatically from public YouTube metadata. ")
	_, _ = fmt.Fprintf(f, "It is not affiliated with or endorsed by %s. Licensed under CC BY-SA 4.0.*\n", ch.Title)
	return nil
}

// --- videos index ---

func exportWriteVideosIndex(videos []Video, fileMap map[string]string, subdir, dir string) error {
	f, err := os.Create(filepath.Join(dir, subdir, "README.md"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	sorted := make([]Video, len(videos))
	copy(sorted, videos)
	sort.Slice(sorted, func(i, j int) bool {
		di, dj := exportVideoDate(sorted[i]), exportVideoDate(sorted[j])
		if di != dj {
			return di > dj
		}
		return sorted[i].ViewCount > sorted[j].ViewCount
	})

	label := "Videos"
	if subdir == "shorts" {
		label = "Shorts"
	}
	_, _ = fmt.Fprintf(f, "# %s\n\n", label)
	_, _ = fmt.Fprintf(f, "[<- Back to channel](../README.md)\n\n")
	_, _ = fmt.Fprintf(f, "%d %s\n\n", len(sorted), strings.ToLower(label))

	curYear, curMonth := "", ""
	for _, v := range sorted {
		d := exportVideoDate(v)
		var y, m string
		if len(d) >= 7 {
			y, m = d[:4], d[5:7]
		} else {
			y, m = "Unknown", ""
		}
		if y != curYear {
			curYear = y
			curMonth = ""
			_, _ = fmt.Fprintf(f, "## %s\n\n", y)
		}
		if m != "" && m != curMonth {
			curMonth = m
			_, _ = fmt.Fprintf(f, "### %s %s\n\n", exportMonthLabel(m), y)
			_, _ = fmt.Fprintf(f, "| Title | Duration | Views |\n|-------|----------|-------|\n")
		}
		rel := fileMap[v.VideoID]
		_, _ = fmt.Fprintf(f, "| [%s](%s.md) | %s | %s |\n",
			exportEscTbl(v.Title), rel, v.DurationText, exportFmtCount(v.ViewCount))
	}
	return nil
}

// --- individual video page ---

func exportWriteVideoPage(v Video, related []Video, chapters []Chapter, fileMap map[string]string, subdir, dir string) error {
	rel := fileMap[v.VideoID]
	path := filepath.Join(dir, subdir, rel+".md")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	toRoot := "../../../../"
	ytURL := "https://www.youtube.com/watch?v=" + v.VideoID

	_, _ = fmt.Fprintf(f, "---\nvideo_id: %s\ntitle: %s\n", v.VideoID, exportYAMLStr(v.Title))
	_, _ = fmt.Fprintf(f, "channel: %s\nchannel_id: %s\n", exportYAMLStr(v.ChannelName), v.ChannelID)
	if v.DurationText != "" {
		_, _ = fmt.Fprintf(f, "duration: %s\n", v.DurationText)
	}
	if v.UploadDate != "" {
		_, _ = fmt.Fprintf(f, "date: %s\n", v.UploadDate)
	}
	_, _ = fmt.Fprintf(f, "views: %d\nlikes: %d\nurl: %s\n---\n\n", v.ViewCount, v.LikeCount, ytURL)

	_, _ = fmt.Fprintf(f, "# %s\n\n", v.Title)
	if v.ThumbnailURL != "" {
		_, _ = fmt.Fprintf(f, "[![Thumbnail](%s)](%s)\n\n", v.ThumbnailURL, ytURL)
	}

	_, _ = fmt.Fprintf(f, "| | |\n|---|---|\n")
	if v.ChannelName != "" {
		_, _ = fmt.Fprintf(f, "| **Channel** | [%s](%sREADME.md) |\n", exportEscTbl(v.ChannelName), toRoot)
	}
	if v.UploadDate != "" {
		_, _ = fmt.Fprintf(f, "| **Published** | %s |\n", v.UploadDate)
	} else if v.PublishedText != "" {
		_, _ = fmt.Fprintf(f, "| **Published** | %s |\n", v.PublishedText)
	}
	if v.DurationText != "" {
		_, _ = fmt.Fprintf(f, "| **Duration** | %s |\n", v.DurationText)
	}
	if v.ViewCount > 0 {
		_, _ = fmt.Fprintf(f, "| **Views** | %s |\n", exportFmtCount(v.ViewCount))
	}
	if v.LikeCount > 0 {
		_, _ = fmt.Fprintf(f, "| **Likes** | %s |\n", exportFmtCount(v.LikeCount))
	}
	if v.CommentCount > 0 {
		_, _ = fmt.Fprintf(f, "| **Comments** | %s |\n", exportFmtCount(v.CommentCount))
	}
	if v.Category != "" {
		_, _ = fmt.Fprintf(f, "| **Category** | %s |\n", v.Category)
	}
	_, _ = fmt.Fprintf(f, "| **YouTube** | [Watch](%s) |\n\n", ytURL)

	if v.Description != "" {
		_, _ = fmt.Fprintf(f, "## Description\n\n%s\n\n", v.Description)
	}

	if len(chapters) > 0 {
		_, _ = fmt.Fprintf(f, "## Chapters\n\n| # | Title | Timestamp |\n|---|-------|-----------|\n")
		for _, ch := range chapters {
			ts := exportFmtTimestamp(ch.StartSeconds)
			_, _ = fmt.Fprintf(f, "| %d | %s | [%s](%s&t=%d) |\n",
				ch.Position, exportEscTbl(ch.Title), ts, ytURL, ch.StartSeconds)
		}
		_, _ = fmt.Fprintln(f)
	}

	if len(v.Tags) > 0 {
		_, _ = fmt.Fprintf(f, "## Tags\n\n")
		for _, tag := range v.Tags {
			_, _ = fmt.Fprintf(f, "`%s` ", tag)
		}
		_, _ = fmt.Fprintf(f, "\n\n")
	}

	if v.Transcript != "" {
		_, _ = fmt.Fprintf(f, "## Transcript\n\n[Read full transcript](%stranscripts/%s.md)\n\n", toRoot, rel)
	}

	if len(related) > 0 {
		_, _ = fmt.Fprintf(f, "## Related Videos\n\n| # | Title | Channel | Views |\n|---|-------|---------|-------|\n")
		for i, r := range related {
			title := r.Title
			if title == "" {
				title = r.VideoID
			}
			_, _ = fmt.Fprintf(f, "| %d | %s | %s | %s |\n",
				i+1, exportVidLink(fileMap, r.VideoID, title, toRoot+"videos/"),
				exportEscTbl(r.ChannelName), exportFmtCount(r.ViewCount))
		}
		_, _ = fmt.Fprintln(f)
	}
	return nil
}

// --- transcript page ---

func exportWriteTranscript(v Video, fileMap map[string]string, dir string) error {
	rel := fileMap[v.VideoID]
	path := filepath.Join(dir, "transcripts", rel+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	toRoot := "../../../../"
	_, _ = fmt.Fprintf(f, "---\nvideo_id: %s\ntitle: %s\n---\n\n", v.VideoID, exportYAMLStr(v.Title))
	_, _ = fmt.Fprintf(f, "# Transcript: %s\n\n", v.Title)
	_, _ = fmt.Fprintf(f, "[<- Back to video](%svideos/%s.md) | [Watch on YouTube](https://www.youtube.com/watch?v=%s)\n\n",
		toRoot, rel, v.VideoID)
	if v.TranscriptLanguage != "" {
		_, _ = fmt.Fprintf(f, "*Language: %s*\n\n", v.TranscriptLanguage)
	}
	_, _ = fmt.Fprintf(f, "---\n\n%s\n", v.Transcript)
	return nil
}

// --- playlists index ---

func exportWritePlaylistsIndex(playlists []Playlist, plItems map[string][]Video, plMap map[string]string, dir string) error {
	f, err := os.Create(filepath.Join(dir, "playlists", "README.md"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	var totalVids int
	var totalViews, totalDur int64
	for _, p := range playlists {
		totalVids += p.VideoCount
		for _, v := range plItems[p.PlaylistID] {
			totalViews += v.ViewCount
			totalDur += int64(v.DurationSeconds)
		}
	}

	_, _ = fmt.Fprintf(f, "# Playlists\n\n")
	_, _ = fmt.Fprintf(f, "[<- Back to channel](../README.md)\n\n")
	_, _ = fmt.Fprintf(f, "**%d playlists** containing %d videos", len(playlists), totalVids)
	if totalViews > 0 {
		_, _ = fmt.Fprintf(f, " with %s combined views", exportFmtCount(totalViews))
	}
	if totalDur > 0 {
		_, _ = fmt.Fprintf(f, " and %s of content", exportFmtDuration(totalDur))
	}
	_, _ = fmt.Fprintf(f, ".\n\n")

	_, _ = fmt.Fprintf(f, "| Title | Videos | Views | Duration |\n|-------|--------|-------|----------|\n")
	for _, p := range playlists {
		slug := plMap[p.PlaylistID]
		items := plItems[p.PlaylistID]
		var views, dur int64
		for _, v := range items {
			views += v.ViewCount
			dur += int64(v.DurationSeconds)
		}
		_, _ = fmt.Fprintf(f, "| [%s](%s.md) | %d | %s | %s |\n",
			exportEscTbl(p.Title), slug, p.VideoCount, exportFmtCount(views), exportFmtDurationShort(dur))
	}
	_, _ = fmt.Fprintln(f)
	return nil
}

// --- individual playlist page ---

func exportWritePlaylistPage(p Playlist, items []Video, fileMap, plMap map[string]string, dir string) error {
	slug := plMap[p.PlaylistID]
	f, err := os.Create(filepath.Join(dir, "playlists", slug+".md"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	ytURL := "https://www.youtube.com/playlist?list=" + p.PlaylistID
	_, _ = fmt.Fprintf(f, "---\nplaylist_id: %s\ntitle: %s\nvideo_count: %d\nurl: %s\n---\n\n",
		p.PlaylistID, exportYAMLStr(p.Title), p.VideoCount, ytURL)

	_, _ = fmt.Fprintf(f, "# %s\n\n", p.Title)
	_, _ = fmt.Fprintf(f, "[<- Back to playlists](README.md) | [Watch on YouTube](%s)\n\n", ytURL)

	if p.Description != "" {
		_, _ = fmt.Fprintf(f, "> %s\n\n", strings.ReplaceAll(strings.TrimSpace(p.Description), "\n", "\n> "))
	}
	_, _ = fmt.Fprintf(f, "**%d videos**", p.VideoCount)
	if p.LastUpdatedText != "" {
		_, _ = fmt.Fprintf(f, " - Last updated %s", p.LastUpdatedText)
	}
	_, _ = fmt.Fprintf(f, "\n\n")

	for i, item := range items {
		title := item.Title
		if title == "" {
			title = item.VideoID
		}
		ytVid := "https://www.youtube.com/watch?v=" + item.VideoID

		_, _ = fmt.Fprintf(f, "---\n\n### %d. %s\n\n", i+1, title)
		if item.ThumbnailURL != "" {
			_, _ = fmt.Fprintf(f, "[![%s](%s)](%s)\n\n", exportEscText(exportTrunc(title, 40)), item.ThumbnailURL, ytVid)
		}

		var meta []string
		if item.DurationText != "" {
			meta = append(meta, "`"+item.DurationText+"`")
		}
		if item.ViewCount > 0 {
			meta = append(meta, exportFmtCount(item.ViewCount)+" views")
		}
		if item.UploadDate != "" {
			meta = append(meta, item.UploadDate)
		} else if item.PublishedText != "" {
			meta = append(meta, item.PublishedText)
		}
		if len(meta) > 0 {
			_, _ = fmt.Fprintf(f, "%s\n\n", strings.Join(meta, " - "))
		}

		if item.Description != "" {
			desc := strings.TrimSpace(item.Description)
			lines := strings.SplitN(desc, "\n", 4)
			if len(lines) > 3 {
				desc = strings.Join(lines[:3], "\n") + "\n..."
			} else {
				desc = strings.Join(lines, "\n")
			}
			_, _ = fmt.Fprintf(f, "> %s\n\n", strings.ReplaceAll(desc, "\n", "\n> "))
		}

		if rel, ok := fileMap[item.VideoID]; ok {
			_, _ = fmt.Fprintf(f, "[Video page](../%s.md) - [YouTube](%s)\n\n", rel, ytVid)
		} else {
			_, _ = fmt.Fprintf(f, "[Watch on YouTube](%s)\n\n", ytVid)
		}
	}
	return nil
}

// --- file path / slug helpers ---

var exportReEpisodePrefix = regexp.MustCompile(`(?i)^(?:(?:S\d{4}\s+)?#\d+|(?:Ep\.?\s*|Episode\s+)\d+)\s*[-:|]+\s*`)

func exportChannelDir(ch Channel) string {
	if ch.Handle != "" {
		h := strings.TrimPrefix(ch.Handle, "@")
		// Strip full URL prefix if present.
		for _, pfx := range []string{
			"https://www.youtube.com/@", "http://www.youtube.com/@",
			"https://youtube.com/@", "http://youtube.com/@",
		} {
			h = strings.TrimPrefix(h, pfx)
		}
		if h != "" {
			return h
		}
	}
	if ch.Title != "" {
		return exportSlugify(ch.Title)
	}
	return ch.ChannelID
}

// exportSlugify converts a string to a URL-safe slug.
func exportSlugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		case c == ' ' || c == '-' || c == '_':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func exportSlugifyTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func exportCleanTitle(title string) string {
	s := title
	if strings.HasPrefix(s, "【") {
		if i := strings.Index(s, "】"); i >= 0 {
			s = strings.TrimSpace(s[i+len("】"):])
		}
	}
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "]"); i >= 0 && i < 80 {
			s = strings.TrimSpace(s[i+1:])
		}
	}
	s = strings.Join(strings.Fields(s), " ")
	for {
		i := strings.LastIndex(s, " | ")
		if i <= 0 {
			break
		}
		suffix := strings.TrimSpace(s[i+3:])
		if len([]rune(suffix)) > len([]rune(s))/2 {
			break
		}
		s = strings.TrimSpace(s[:i])
	}
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, ")") {
		depth := 0
		matchStart := -1
		runes := []rune(s)
		for i := len(runes) - 1; i >= 0; i-- {
			switch runes[i] {
			case ')':
				depth++
			case '(':
				depth--
				if depth == 0 {
					matchStart = i
				}
			}
			if matchStart >= 0 {
				break
			}
		}
		if matchStart > 0 && matchStart > len([]rune(s))/3 {
			s = strings.TrimSpace(string([]rune(s)[:matchStart]))
		}
	}
	s = exportReEpisodePrefix.ReplaceAllString(s, "")
	return strings.TrimRight(strings.TrimSpace(s), " .,;:!?-")
}

func exportVideoSlug(v Video) string {
	title := exportCleanTitle(v.Title)
	slug := exportSlugifyTitle(title)
	if slug == "" {
		slug = v.VideoID
	}
	runes := []rune(slug)
	if len(runes) > 60 {
		s := string(runes[:60])
		if i := strings.LastIndex(s, "-"); i > len(s)/2 {
			slug = s[:i]
		} else {
			slug = s
		}
	}
	return slug + "-" + v.VideoID
}

func exportVideoRelPath(v Video) string {
	d := exportVideoDate(v)
	parts := strings.SplitN(d, "-", 3)
	var dateDir string
	if len(parts) == 3 {
		dateDir = parts[0] + "/" + parts[1] + "/" + parts[2]
	} else {
		dateDir = "0000/00/00"
	}
	return dateDir + "/" + exportVideoSlug(v)
}

func exportVideoDate(v Video) string {
	if v.UploadDate != "" {
		if t, err := time.Parse(time.RFC3339, v.UploadDate); err == nil {
			return t.Format("2006-01-02")
		}
		if t, err := time.Parse("2006-01-02", v.UploadDate); err == nil {
			return t.Format("2006-01-02")
		}
	}
	if !v.PublishedAt.IsZero() {
		return v.PublishedAt.Format("2006-01-02")
	}
	if d := exportParseRelDate(v.PublishedText); d != "" {
		return d
	}
	return "0000-00-00"
}

func exportParseRelDate(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return ""
	}
	text = strings.TrimPrefix(text, "streamed ")
	parts := strings.Fields(text)
	if len(parts) < 3 || parts[len(parts)-1] != "ago" {
		return ""
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return ""
	}
	now := time.Now()
	unit := parts[1]
	switch {
	case strings.HasPrefix(unit, "second"), strings.HasPrefix(unit, "minute"), strings.HasPrefix(unit, "hour"):
		return now.Format("2006-01-02")
	case strings.HasPrefix(unit, "day"):
		return now.AddDate(0, 0, -n).Format("2006-01-02")
	case strings.HasPrefix(unit, "week"):
		return now.AddDate(0, 0, -7*n).Format("2006-01-02")
	case strings.HasPrefix(unit, "month"):
		return now.AddDate(0, -n, 0).Format("2006-01-02")
	case strings.HasPrefix(unit, "year"):
		return now.AddDate(-n, 0, 0).Format("2006-01-02")
	}
	return ""
}

func exportPlaylistSlug(p Playlist) string {
	title := exportCleanTitle(p.Title)
	slug := exportSlugifyTitle(title)
	if slug == "" {
		slug = p.PlaylistID
	}
	runes := []rune(slug)
	if len(runes) > 60 {
		s := string(runes[:60])
		if i := strings.LastIndex(s, "-"); i > len(s)/2 {
			slug = s[:i]
		} else {
			slug = s
		}
	}
	suffix := p.PlaylistID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return fmt.Sprintf("%s-%s", slug, suffix)
}

// --- formatting helpers ---

func exportFmtCount(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	case n > 0:
		return fmt.Sprintf("%d", n)
	default:
		return "-"
	}
}

func exportFmtDuration(totalSeconds int64) string {
	h := totalSeconds / 3600
	if h >= 24 {
		d := h / 24
		rem := h % 24
		return fmt.Sprintf("%dd %dh", d, rem)
	}
	m := (totalSeconds % 3600) / 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func exportFmtDurationShort(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	if seconds >= 3600 {
		h := seconds / 3600
		m := (seconds % 3600) / 60
		return fmt.Sprintf("%dh %dm", h, m)
	}
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%dm %ds", m, s)
}

func exportFmtTimestamp(secs int) string {
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

var exportMonths = [...]string{"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December"}

func exportMonthLabel(m string) string {
	if n, err := strconv.Atoi(m); err == nil && n >= 1 && n <= 12 {
		return exportMonths[n]
	}
	return m
}

func exportEscTbl(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

func exportEscText(s string) string {
	s = strings.ReplaceAll(s, "[", "\\[")
	return strings.ReplaceAll(s, "]", "\\]")
}

func exportTrunc(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-3]) + "..."
}

func exportYAMLStr(s string) string {
	if s == "" {
		return `""`
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func exportStatRow(f *os.File, label, value string) {
	if value != "" {
		_, _ = fmt.Fprintf(f, "| **%s** | %s |\n", label, value)
	}
}

func exportVidLink(fileMap map[string]string, videoID, title, prefix string) string {
	esc := exportEscTbl(exportTrunc(title, 60))
	if slug, ok := fileMap[videoID]; ok {
		return fmt.Sprintf("[%s](%s%s.md)", esc, prefix, slug)
	}
	return fmt.Sprintf("[%s](https://www.youtube.com/watch?v=%s)", esc, videoID)
}

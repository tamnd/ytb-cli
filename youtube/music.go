package youtube

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	musicBaseURL = "https://music.youtube.com"
)

// musicSearchParams returns the WEB_REMIX filter param for a given type string.
func musicSearchParams(typ string) string {
	switch strings.ToLower(typ) {
	case "song":
		return "EgWKAQIIAWoMEA4QChADEAQQCRAF"
	case "video":
		return "EgWKAQIQAWoMEA4QChADEAQQCRAF"
	case "album":
		return "EgWKAQIYAWoMEA4QChADEAQQCRAF"
	case "artist":
		return "EgWKAQIgAWoMEA4QChADEAQQCRAF"
	case "playlist":
		return "EgWKAQIoAWoMEA4QChADEAQQCRAF"
	default:
		return ""
	}
}

// musicBrowseID extracts or normalises a music browse/channel/playlist ID from
// a music.youtube.com URL or a raw ID. Returns the ID to pass to MusicBrowse.
func musicBrowseID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	// Already a bare ID.
	for _, prefix := range []string{"UC", "MPRE", "VL", "PL", "RDCLAK", "OLAK"} {
		if strings.HasPrefix(input, prefix) && !strings.Contains(input, "/") {
			return input
		}
	}
	// music.youtube.com/browse/<id>
	if strings.Contains(input, "music.youtube.com/browse/") {
		if u, err := url.Parse(input); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 && parts[0] == "browse" {
				return parts[1]
			}
		}
	}
	// music.youtube.com/channel/<id>
	if strings.Contains(input, "music.youtube.com/channel/") {
		if u, err := url.Parse(input); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) >= 2 && parts[0] == "channel" {
				return parts[1]
			}
		}
	}
	// music.youtube.com/playlist?list=<id>
	if strings.Contains(input, "music.youtube.com/playlist") {
		if u, err := url.Parse(input); err == nil {
			if id := u.Query().Get("list"); id != "" {
				return id
			}
		}
	}
	// watch?v= — treat as a video/song ID
	if strings.Contains(input, "watch") {
		if u, err := url.Parse(input); err == nil {
			if v := u.Query().Get("v"); v != "" {
				return v
			}
		}
	}
	return input
}

// MusicSearch streams search results for the given query and type filter.
// typ may be "", "song", "album", "artist", "playlist", or "video".
// emit receives Artist, Album, or Song values. Iteration stops on ErrStop.
func (c *Client) MusicSearch(ctx context.Context, query, typ string, opt PageOptions, emit func(any) error) error {
	it := NewInnerTube(c)
	params := musicSearchParams(typ)

	total := 0
	pages := 0
	continuation := ""

	for opt.MaxPages <= 0 || pages < opt.MaxPages {
		var (
			data map[string]any
			err  error
		)
		if continuation == "" {
			data, err = it.MusicSearch(ctx, query, params, "")
		} else {
			data, err = it.MusicSearch(ctx, query, params, continuation)
		}
		if err != nil {
			return err
		}
		pages++

		done, emitted, err := musicEmitSearchResults(data, typ, emit, opt.Max-total)
		total += emitted
		if err != nil {
			if errors.Is(err, ErrStop) {
				return nil
			}
			return err
		}
		if done {
			break
		}
		if opt.Max > 0 && total >= opt.Max {
			break
		}

		continuation = musicExtractContinuation(data)
		if continuation == "" {
			break
		}
	}
	return nil
}

// musicEmitSearchResults walks a /search response and emits typed results.
// It returns (done, count, error). limit <= 0 means unlimited.
func musicEmitSearchResults(data map[string]any, typ string, emit func(any) error, limit int) (bool, int, error) {
	count := 0
	var emitErr error

	walkJSON(data, func(m map[string]any) {
		if emitErr != nil {
			return
		}
		if limit > 0 && count >= limit {
			return
		}

		if r, ok := m["musicShelfRenderer"].(map[string]any); ok {
			title := strings.ToLower(extractText(r["title"]))
			contents, _ := r["contents"].([]any)
			for _, item := range contents {
				if emitErr != nil {
					break
				}
				if limit > 0 && count >= limit {
					break
				}
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				mrlir, ok := im["musicResponsiveListItemRenderer"].(map[string]any)
				if !ok {
					continue
				}
				val := musicParseListItem(mrlir, title, typ)
				if val == nil {
					continue
				}
				if err := emit(val); err != nil {
					emitErr = err
					return
				}
				count++
			}
		}

		// Top-result card (musicCardShelfRenderer) - song type.
		if r, ok := m["musicCardShelfRenderer"].(map[string]any); ok {
			if limit > 0 && count >= limit {
				return
			}
			s := musicParseCardShelf(r)
			if s.VideoID != "" && (typ == "" || typ == "song") {
				if err := emit(s); err != nil {
					emitErr = err
					return
				}
				count++
			}
		}
	})

	if errors.Is(emitErr, ErrStop) {
		return true, count, ErrStop
	}
	return false, count, emitErr
}

// musicParseListItem decides which type to parse from a musicResponsiveListItemRenderer
// based on the section title hint and explicit type filter.
func musicParseListItem(r map[string]any, sectionTitle, typ string) any {
	tl := strings.ToLower(sectionTitle)
	filter := strings.ToLower(typ)

	wantsArtist := filter == "artist" || (filter == "" && strings.Contains(tl, "artist"))
	wantsAlbum := filter == "album" || (filter == "" && (strings.Contains(tl, "album") || strings.Contains(tl, "single") || strings.Contains(tl, "ep")))
	wantsPlaylist := filter == "playlist" || (filter == "" && strings.Contains(tl, "playlist"))

	switch {
	case wantsArtist:
		art := parseMusicListItemArtist(r)
		if art.ArtistID != "" || art.Name != "" {
			return art
		}
	case wantsAlbum:
		alb := parseMusicListItemAlbum(r)
		if alb.AlbumID != "" || alb.Title != "" {
			return alb
		}
	case wantsPlaylist:
		// Return Album used as playlist container.
		alb := parseMusicListItemPlaylist(r)
		if alb.AlbumID != "" || alb.Title != "" {
			return alb
		}
	default:
		// song / video / unknown
		s := parseMusicListItemSong(r)
		if s.VideoID != "" {
			if filter == "video" || strings.Contains(tl, "video") {
				s.VideoType = "OMV"
			}
			return s
		}
	}
	return nil
}

// parseMusicListItemSong parses a musicResponsiveListItemRenderer into a Song.
func parseMusicListItemSong(r map[string]any) Song {
	s := Song{FetchedAt: time.Now()}

	// Video ID from watchEndpoint.
	walkJSON(r, func(m map[string]any) {
		if s.VideoID != "" {
			return
		}
		if wep, ok := m["watchEndpoint"].(map[string]any); ok {
			if v := stringValue(wep["videoId"]); v != "" {
				s.VideoID = v
			}
		}
	})
	if s.VideoID != "" {
		s.URL = musicBaseURL + "/watch?v=" + s.VideoID
	}

	cols := musicExtractFlexColumns(r)
	if len(cols) > 0 {
		s.Title = cols[0]
	}
	if len(cols) > 1 {
		raw := cols[1]
		if musicLooksLikePlays(raw) {
			s.PlaysText = raw
		} else {
			parts := musicSplitDot(raw)
			for i, p := range parts {
				p = strings.TrimSpace(p)
				if musicLooksLikePlays(p) {
					s.PlaysText = p
					continue
				}
				if i == 0 && s.ArtistName == "" {
					s.ArtistName = p
				} else if i == 1 && s.AlbumName == "" {
					s.AlbumName = p
				}
			}
		}
	}
	for i := 2; i < len(cols); i++ {
		col := strings.TrimSpace(cols[i])
		if col == "" {
			continue
		}
		if musicLooksLikePlays(col) && s.PlaysText == "" {
			s.PlaysText = col
		} else if strings.Contains(col, ":") && s.DurationText == "" {
			s.DurationText = col
			s.DurationSeconds = parseDurationSeconds(col)
		}
	}

	// Fixed columns - duration is authoritative here.
	for _, fc := range musicExtractFixedColumns(r) {
		fc = strings.TrimSpace(fc)
		if strings.Contains(fc, ":") {
			s.DurationText = fc
			s.DurationSeconds = parseDurationSeconds(fc)
		} else if musicLooksLikePlays(fc) && s.PlaysText == "" {
			s.PlaysText = fc
		}
	}

	// Thumbnail.
	s.ThumbnailURL = musicExtractThumbnail(r)

	// Explicit badge.
	walkJSON(r, func(m map[string]any) {
		if _, ok := m["musicInlineBadgeRenderer"].(map[string]any); ok {
			s.IsExplicit = true
		}
	})

	// Artist browse ID.
	walkJSON(r, func(m map[string]any) {
		if s.ArtistID != "" {
			return
		}
		if ep, ok := m["browseEndpoint"].(map[string]any); ok {
			bid := stringValue(ep["browseId"])
			pt := musicPageType(ep)
			if strings.HasPrefix(bid, "UC") || pt == "MUSIC_PAGE_TYPE_ARTIST" {
				s.ArtistID = bid
			}
		}
	})

	// Album browse ID.
	walkJSON(r, func(m map[string]any) {
		if s.AlbumID != "" {
			return
		}
		if ep, ok := m["browseEndpoint"].(map[string]any); ok {
			bid := stringValue(ep["browseId"])
			pt := musicPageType(ep)
			if strings.HasPrefix(bid, "MPRE") || pt == "MUSIC_PAGE_TYPE_ALBUM" {
				s.AlbumID = bid
			}
		}
	})

	return s
}

// parseMusicListItemAlbum parses a musicResponsiveListItemRenderer into an Album.
func parseMusicListItemAlbum(r map[string]any) Album {
	alb := Album{FetchedAt: time.Now()}

	cols := musicExtractFlexColumns(r)
	if len(cols) > 0 {
		alb.Title = cols[0]
	}
	if len(cols) > 1 {
		for _, p := range musicSplitDot(cols[1]) {
			p = strings.TrimSpace(p)
			pl := strings.ToLower(p)
			switch {
			case pl == "album" || pl == "single" || pl == "ep":
				alb.AlbumType = p
			case musicIsYear(p):
				alb.Year = p
			case alb.ArtistName == "":
				alb.ArtistName = p
			}
		}
	}

	walkJSON(r, func(m map[string]any) {
		if alb.AlbumID != "" {
			return
		}
		if ep, ok := m["browseEndpoint"].(map[string]any); ok {
			bid := stringValue(ep["browseId"])
			if strings.HasPrefix(bid, "MPRE") {
				alb.AlbumID = bid
			}
		}
	})
	if alb.AlbumID != "" {
		alb.URL = musicBaseURL + "/browse/" + alb.AlbumID
	}

	alb.ThumbnailURL = musicExtractThumbnail(r)
	return alb
}

// parseMusicListItemArtist parses a musicResponsiveListItemRenderer into an Artist.
func parseMusicListItemArtist(r map[string]any) Artist {
	art := Artist{FetchedAt: time.Now()}

	cols := musicExtractFlexColumns(r)
	if len(cols) > 0 {
		art.Name = cols[0]
	}
	if len(cols) > 1 {
		art.SubscribersText = cols[1]
	}

	walkJSON(r, func(m map[string]any) {
		if art.ArtistID != "" {
			return
		}
		if ep, ok := m["browseEndpoint"].(map[string]any); ok {
			bid := stringValue(ep["browseId"])
			if strings.HasPrefix(bid, "UC") {
				art.ArtistID = bid
			}
		}
	})
	if art.ArtistID != "" {
		art.URL = musicBaseURL + "/channel/" + art.ArtistID
	}
	art.ThumbnailURL = musicExtractThumbnail(r)
	return art
}

// parseMusicListItemPlaylist parses a musicResponsiveListItemRenderer into an Album
// used as a playlist container.
func parseMusicListItemPlaylist(r map[string]any) Album {
	alb := Album{FetchedAt: time.Now(), AlbumType: "Playlist"}

	cols := musicExtractFlexColumns(r)
	if len(cols) > 0 {
		alb.Title = cols[0]
	}
	if len(cols) > 1 {
		alb.ArtistName = cols[1]
	}

	walkJSON(r, func(m map[string]any) {
		if alb.AlbumID != "" {
			return
		}
		if ep, ok := m["browseEndpoint"].(map[string]any); ok {
			bid := stringValue(ep["browseId"])
			if strings.HasPrefix(bid, "VL") {
				alb.AudioPlaylistID = strings.TrimPrefix(bid, "VL")
				alb.AlbumID = bid
			}
		}
		if wep, ok := m["watchPlaylistEndpoint"].(map[string]any); ok {
			if pid := stringValue(wep["playlistId"]); pid != "" && alb.AudioPlaylistID == "" {
				alb.AudioPlaylistID = pid
				alb.AlbumID = "VL" + pid
			}
		}
	})
	if alb.AudioPlaylistID != "" {
		alb.URL = musicBaseURL + "/playlist?list=" + alb.AudioPlaylistID
	}

	alb.ThumbnailURL = musicExtractThumbnail(r)
	return alb
}

// musicParseCardShelf parses a musicCardShelfRenderer (top-result card) into a Song.
func musicParseCardShelf(r map[string]any) Song {
	s := Song{FetchedAt: time.Now()}
	s.Title = extractText(r["title"])

	walkJSON(r, func(m map[string]any) {
		if s.VideoID != "" {
			return
		}
		if wep, ok := m["watchEndpoint"].(map[string]any); ok {
			if v := stringValue(wep["videoId"]); v != "" {
				s.VideoID = v
			}
		}
	})
	if s.VideoID != "" {
		s.URL = musicBaseURL + "/watch?v=" + s.VideoID
	}

	if sub, ok := r["subtitle"]; ok {
		for _, p := range musicExtractRunTexts(sub) {
			p = strings.TrimSpace(p)
			pl := strings.ToLower(p)
			if p == "" || p == "•" || p == "·" || pl == "song" || pl == "video" {
				continue
			}
			if s.ArtistName == "" {
				s.ArtistName = p
			}
		}
	}

	s.ThumbnailURL = musicExtractThumbnail(r)
	return s
}

// FetchArtist fetches the artist page for the given ID or URL and returns the
// artist, their albums (including singles/EPs), and top songs.
func (c *Client) FetchArtist(ctx context.Context, idOrURL string) (*Artist, []Album, []Song, error) {
	browseID := musicBrowseID(idOrURL)
	if browseID == "" {
		return nil, nil, nil, errors.New("FetchArtist: empty browse ID")
	}

	it := NewInnerTube(c)
	data, err := it.MusicBrowse(ctx, browseID, "", "")
	if err != nil {
		return nil, nil, nil, err
	}

	artist := &Artist{FetchedAt: time.Now()}

	// Header parsers.
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicImmersiveHeaderRenderer"].(map[string]any); ok {
			if artist.Name == "" {
				artist.Name = extractText(r["title"])
			}
			if artist.SubscribersText == "" {
				artist.SubscribersText = extractText(r["subscriptionButton"])
			}
			if artist.ThumbnailURL == "" {
				if thumb, ok := r["thumbnail"].(map[string]any); ok {
					if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
						if t, ok := tvm["thumbnail"].(map[string]any); ok {
							artist.ThumbnailURL = bestThumbnail(t["thumbnails"])
						}
					}
				}
			}
		}
		if r, ok := m["musicVisualHeaderRenderer"].(map[string]any); ok {
			if artist.Name == "" {
				artist.Name = extractText(r["title"])
			}
			if artist.SubscribersText == "" {
				artist.SubscribersText = extractText(r["subtitle"])
			}
		}
		if r, ok := m["musicResponsiveHeaderRenderer"].(map[string]any); ok {
			if artist.Name == "" {
				artist.Name = extractText(r["title"])
			}
			if artist.SubscribersText == "" {
				artist.SubscribersText = extractText(r["subtitle"])
			}
			if artist.ThumbnailURL == "" {
				if thumb, ok := r["thumbnail"].(map[string]any); ok {
					if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
						if t, ok := tvm["thumbnail"].(map[string]any); ok {
							artist.ThumbnailURL = bestThumbnail(t["thumbnails"])
						}
					}
				}
			}
		}
		if r, ok := m["musicDescriptionShelfRenderer"].(map[string]any); ok {
			if artist.Description == "" {
				artist.Description = extractText(r["description"])
			}
		}
	})

	// Artist ID from subscribeButtonRenderer or browseId.
	walkJSON(data, func(m map[string]any) {
		if artist.ArtistID != "" {
			return
		}
		if r, ok := m["subscribeButtonRenderer"].(map[string]any); ok {
			if ch := stringValue(r["channelId"]); ch != "" {
				artist.ArtistID = ch
			}
		}
	})
	if artist.ArtistID == "" {
		walkJSON(data, func(m map[string]any) {
			if artist.ArtistID != "" {
				return
			}
			if bid := stringValue(m["browseId"]); strings.HasPrefix(bid, "UC") {
				artist.ArtistID = bid
			}
		})
	}
	// Fall back to the input ID if it looks like a channel.
	if artist.ArtistID == "" && strings.HasPrefix(browseID, "UC") {
		artist.ArtistID = browseID
	}
	if artist.ArtistID != "" {
		artist.URL = musicBaseURL + "/channel/" + artist.ArtistID
	}

	var albums []Album
	var songs []Song

	// Walk carousels for songs, albums, singles.
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicCarouselShelfRenderer"].(map[string]any); ok {
			title := strings.ToLower(musicCarouselTitle(r))
			contents, _ := r["contents"].([]any)

			for _, item := range contents {
				im, ok := item.(map[string]any)
				if !ok {
					continue
				}
				switch {
				case strings.Contains(title, "song"):
					if mrlir, ok := im["musicResponsiveListItemRenderer"].(map[string]any); ok {
						s := parseMusicListItemSong(mrlir)
						if s.VideoID != "" {
							if s.ArtistID == "" {
								s.ArtistID = artist.ArtistID
							}
							if s.ArtistName == "" {
								s.ArtistName = artist.Name
							}
							songs = append(songs, s)
						}
					}
				case strings.Contains(title, "video"):
					if mrlir, ok := im["musicResponsiveListItemRenderer"].(map[string]any); ok {
						s := parseMusicListItemSong(mrlir)
						if s.VideoID != "" {
							s.VideoType = "OMV"
							if s.ArtistID == "" {
								s.ArtistID = artist.ArtistID
							}
							if s.ArtistName == "" {
								s.ArtistName = artist.Name
							}
							songs = append(songs, s)
						}
					}
					if mtrir, ok := im["musicTwoRowItemRenderer"].(map[string]any); ok {
						s := musicParseTwoRowSong(mtrir)
						if s.VideoID != "" {
							s.VideoType = "OMV"
							if s.ArtistID == "" {
								s.ArtistID = artist.ArtistID
							}
							if s.ArtistName == "" {
								s.ArtistName = artist.Name
							}
							songs = append(songs, s)
						}
					}
				case strings.Contains(title, "album") || strings.Contains(title, "single") || strings.Contains(title, "ep"):
					if mtrir, ok := im["musicTwoRowItemRenderer"].(map[string]any); ok {
						alb := musicParseTwoRowAlbum(mtrir)
						if alb.AlbumID != "" || alb.Title != "" {
							if alb.ArtistID == "" {
								alb.ArtistID = artist.ArtistID
							}
							if alb.ArtistName == "" {
								alb.ArtistName = artist.Name
							}
							if strings.Contains(title, "single") || strings.Contains(title, "ep") {
								if alb.AlbumType == "" {
									alb.AlbumType = "Single"
								}
							} else {
								if alb.AlbumType == "" {
									alb.AlbumType = "Album"
								}
							}
							albums = append(albums, alb)
						}
					}
				}
			}
		}
	})

	return artist, albums, songs, nil
}

// FetchAlbum fetches the album page for the given ID or URL and returns
// the album and its track list.
func (c *Client) FetchAlbum(ctx context.Context, idOrURL string) (*Album, []Song, error) {
	browseID := musicBrowseID(idOrURL)
	if browseID == "" {
		return nil, nil, errors.New("FetchAlbum: empty browse ID")
	}

	it := NewInnerTube(c)
	data, err := it.MusicBrowse(ctx, browseID, "", "")
	if err != nil {
		return nil, nil, err
	}

	alb := &Album{
		AlbumID:   browseID,
		URL:       musicBaseURL + "/browse/" + browseID,
		FetchedAt: time.Now(),
	}

	// Parse header variants.
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicResponsiveHeaderRenderer"].(map[string]any); ok {
			if alb.Title == "" {
				alb.Title = extractText(r["title"])
			}
			if sub, ok := r["subtitle"]; ok {
				for _, p := range musicExtractRunTexts(sub) {
					p = strings.TrimSpace(p)
					pl := strings.ToLower(p)
					switch {
					case pl == "album" || pl == "single" || pl == "ep":
						if alb.AlbumType == "" {
							alb.AlbumType = p
						}
					case musicIsYear(p):
						if alb.Year == "" {
							alb.Year = p
						}
					}
				}
			}
			if strapline, ok := r["straplineTextOne"]; ok {
				if alb.ArtistName == "" {
					alb.ArtistName = extractText(strapline)
				}
				walkJSON(strapline, func(inner map[string]any) {
					if alb.ArtistID != "" {
						return
					}
					if ep, ok := inner["browseEndpoint"].(map[string]any); ok {
						if bid := stringValue(ep["browseId"]); strings.HasPrefix(bid, "UC") {
							alb.ArtistID = bid
						}
					}
				})
			}
			if alb.ThumbnailURL == "" {
				if thumb, ok := r["thumbnail"].(map[string]any); ok {
					if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
						if t, ok := tvm["thumbnail"].(map[string]any); ok {
							alb.ThumbnailURL = bestThumbnail(t["thumbnails"])
						}
					}
				}
			}
			// audioPlaylistId from menu endpoints.
			walkJSON(r, func(inner map[string]any) {
				if alb.AudioPlaylistID != "" {
					return
				}
				if wep, ok := inner["watchPlaylistEndpoint"].(map[string]any); ok {
					alb.AudioPlaylistID = stringValue(wep["playlistId"])
				}
			})
		}
		if r, ok := m["musicDetailHeaderRenderer"].(map[string]any); ok {
			if alb.Title == "" {
				alb.Title = extractText(r["title"])
			}
			if sub, ok := r["subtitle"]; ok {
				for _, p := range musicExtractRunTexts(sub) {
					p = strings.TrimSpace(p)
					pl := strings.ToLower(p)
					switch {
					case pl == "album" || pl == "single" || pl == "ep":
						if alb.AlbumType == "" {
							alb.AlbumType = p
						}
					case musicIsYear(p):
						if alb.Year == "" {
							alb.Year = p
						}
					}
				}
			}
			if alb.ArtistName == "" {
				alb.ArtistName = extractText(r["byline"])
			}
			if alb.Description == "" {
				alb.Description = extractText(r["description"])
			}
			if alb.ThumbnailURL == "" {
				if thumb, ok := r["thumbnail"].(map[string]any); ok {
					if crumb, ok := thumb["croppedSquareThumbnailRenderer"].(map[string]any); ok {
						if t, ok := crumb["thumbnail"].(map[string]any); ok {
							alb.ThumbnailURL = bestThumbnail(t["thumbnails"])
						}
					}
				}
			}
			if alb.AudioPlaylistID == "" {
				if menu, ok := r["menu"].(map[string]any); ok {
					walkJSON(menu, func(inner map[string]any) {
						if alb.AudioPlaylistID != "" {
							return
						}
						if wep, ok := inner["watchPlaylistEndpoint"].(map[string]any); ok {
							alb.AudioPlaylistID = stringValue(wep["playlistId"])
						}
					})
				}
			}
		}
	})

	// Fallback: find audioPlaylistId anywhere.
	if alb.AudioPlaylistID == "" {
		walkJSON(data, func(m map[string]any) {
			if alb.AudioPlaylistID != "" {
				return
			}
			if wep, ok := m["watchPlaylistEndpoint"].(map[string]any); ok {
				alb.AudioPlaylistID = stringValue(wep["playlistId"])
			}
		})
	}

	// Parse tracks.
	var songs []Song
	pos := 0
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicResponsiveListItemRenderer"].(map[string]any); ok {
			s := parseMusicListItemSong(r)
			if s.VideoID != "" {
				pos++
				if s.AlbumID == "" {
					s.AlbumID = browseID
				}
				if s.AlbumName == "" {
					s.AlbumName = alb.Title
				}
				if s.ArtistID == "" {
					s.ArtistID = alb.ArtistID
				}
				if s.ArtistName == "" {
					s.ArtistName = alb.ArtistName
				}
				songs = append(songs, s)
			}
		}
	})
	alb.TrackCount = len(songs)

	return alb, songs, nil
}

// FetchMusicPlaylist fetches a music playlist by ID or URL and returns
// an Album (playlist metadata) and the song list.
func (c *Client) FetchMusicPlaylist(ctx context.Context, idOrURL string) (*Album, []Song, error) {
	rawID := musicBrowseID(idOrURL)
	if rawID == "" {
		return nil, nil, errors.New("FetchMusicPlaylist: empty playlist ID")
	}

	// Playlists browse under VL<playlistId>.
	browseID := rawID
	if !strings.HasPrefix(rawID, "VL") {
		browseID = "VL" + rawID
	}
	playlistID := strings.TrimPrefix(rawID, "VL")

	it := NewInnerTube(c)
	data, err := it.MusicBrowse(ctx, browseID, "", "")
	if err != nil {
		return nil, nil, err
	}

	alb := &Album{
		AlbumID:         browseID,
		AudioPlaylistID: playlistID,
		URL:             musicBaseURL + "/playlist?list=" + playlistID,
		AlbumType:       "Playlist",
		FetchedAt:       time.Now(),
	}

	// Parse header.
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicResponsiveHeaderRenderer"].(map[string]any); ok {
			if alb.Title == "" {
				alb.Title = extractText(r["title"])
			}
			if alb.Description == "" {
				alb.Description = extractText(r["subtitle"])
			}
			if alb.ArtistName == "" {
				alb.ArtistName = extractText(r["straplineTextOne"])
			}
			if alb.ThumbnailURL == "" {
				if thumb, ok := r["thumbnail"].(map[string]any); ok {
					if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
						if t, ok := tvm["thumbnail"].(map[string]any); ok {
							alb.ThumbnailURL = bestThumbnail(t["thumbnails"])
						}
					}
				}
			}
		}
		if r, ok := m["musicDetailHeaderRenderer"].(map[string]any); ok {
			if alb.Title == "" {
				alb.Title = extractText(r["title"])
			}
			if alb.Description == "" {
				alb.Description = extractText(r["description"])
			}
			if alb.ArtistName == "" {
				alb.ArtistName = extractText(r["byline"])
			}
		}
		if r, ok := m["musicEditablePlaylistDetailHeaderRenderer"].(map[string]any); ok {
			if inner, ok := r["header"].(map[string]any); ok {
				if dhr, ok := inner["musicResponsiveHeaderRenderer"].(map[string]any); ok {
					if alb.Title == "" {
						alb.Title = extractText(dhr["title"])
					}
				}
				if dhr, ok := inner["musicDetailHeaderRenderer"].(map[string]any); ok {
					if alb.Title == "" {
						alb.Title = extractText(dhr["title"])
					}
				}
			}
		}
	})

	var songs []Song
	pos := 0
	walkJSON(data, func(m map[string]any) {
		if r, ok := m["musicResponsiveListItemRenderer"].(map[string]any); ok {
			s := parseMusicListItemSong(r)
			if s.VideoID != "" {
				pos++
				songs = append(songs, s)
			}
		}
	})
	alb.TrackCount = len(songs)

	// Handle continuation for large playlists.
	contToken := musicExtractContinuation(data)
	for contToken != "" {
		nextData, err := it.MusicBrowse(ctx, "", "", contToken)
		if err != nil {
			break
		}
		walkJSON(nextData, func(m map[string]any) {
			if r, ok := m["musicResponsiveListItemRenderer"].(map[string]any); ok {
				s := parseMusicListItemSong(r)
				if s.VideoID != "" {
					pos++
					songs = append(songs, s)
				}
			}
		})
		alb.TrackCount = len(songs)
		contToken = musicExtractContinuation(nextData)
	}

	return alb, songs, nil
}

// FetchSong fetches details for a single song/video via MusicPlayer and
// optionally retrieves lyrics via the /next browse tab.
func (c *Client) FetchSong(ctx context.Context, videoID string, withLyrics bool) (*Song, error) {
	if videoID == "" {
		return nil, errors.New("FetchSong: empty video ID")
	}

	it := NewInnerTube(c)
	data, err := it.MusicPlayer(ctx, videoID)
	if err != nil {
		return nil, err
	}

	s := &Song{
		VideoID:   videoID,
		URL:       musicBaseURL + "/watch?v=" + videoID,
		FetchedAt: time.Now(),
	}

	if details, ok := data["videoDetails"].(map[string]any); ok {
		s.Title = stringValue(details["title"])
		s.ArtistName = stringValue(details["author"])
		s.DurationSeconds = int(int64Value(details["lengthSeconds"]))
		if vc := stringValue(details["viewCount"]); vc != "" {
			s.PlaysText = vc + " plays"
		}
		if mvt := stringValue(details["musicVideoType"]); mvt != "" {
			switch mvt {
			case "MUSIC_VIDEO_TYPE_ATV":
				s.VideoType = "ATV"
			case "MUSIC_VIDEO_TYPE_OMV":
				s.VideoType = "OMV"
			case "MUSIC_VIDEO_TYPE_UGC":
				s.VideoType = "UGC"
			default:
				s.VideoType = mvt
			}
		}
		if thumbs, ok := details["thumbnail"].(map[string]any); ok {
			s.ThumbnailURL = bestThumbnail(thumbs["thumbnails"])
		}
		// DurationText from seconds.
		if s.DurationSeconds > 0 && s.DurationText == "" {
			m := s.DurationSeconds / 60
			sec := s.DurationSeconds % 60
			if m >= 60 {
				h := m / 60
				m = m % 60
				s.DurationText = strings.TrimLeft(strings.Join([]string{
					padTwo(h), padTwo(m), padTwo(sec),
				}, ":"), "0")
			} else {
				s.DurationText = padTwo(m) + ":" + padTwo(sec)
			}
		}
	}

	if !withLyrics {
		return s, nil
	}

	// Fetch lyrics via /next -> lyrics tab browseId.
	nextData, err := it.MusicBrowse(ctx, "", "", "")
	if err == nil {
		_ = nextData
	}
	// Use a separate /next-style call to retrieve the lyrics browse ID.
	nextResp, err := c.postJSON(ctx, musicInnertubeURL+"/next", map[string]any{
		"context":                       NewInnerTube(c).musicContext(),
		"videoId":                       videoID,
		"isAudioOnly":                   true,
		"enablePersistentPlaylistPanel": true,
	})
	if err == nil {
		lyricsID := musicParseLyricsBrowseID(nextResp)
		if lyricsID != "" {
			lyricsData, lerr := it.MusicBrowse(ctx, lyricsID, "", "")
			if lerr == nil {
				s.Lyrics = musicParseLyricsPage(lyricsData)
			}
		}
	}

	return s, nil
}

// musicParseLyricsBrowseID extracts the lyrics browseId from a /next response.
func musicParseLyricsBrowseID(data map[string]any) string {
	var browseID string
	walkJSON(data, func(m map[string]any) {
		if browseID != "" {
			return
		}
		if r, ok := m["tabRenderer"].(map[string]any); ok {
			title := strings.ToLower(extractText(r["title"]))
			if !strings.Contains(title, "lyric") {
				return
			}
			tryEndpoints := func(ep map[string]any) {
				if ep == nil {
					return
				}
				if be, ok := ep["browseEndpoint"].(map[string]any); ok {
					if bid := stringValue(be["browseId"]); bid != "" && browseID == "" {
						browseID = bid
					}
				}
			}
			if ep, ok := r["endpoint"].(map[string]any); ok {
				tryEndpoints(ep)
			}
			if ep, ok := r["unselectedEndpoint"].(map[string]any); ok {
				tryEndpoints(ep)
			}
		}
	})
	return browseID
}

// musicParseLyricsPage extracts lyrics text from a lyrics browse response.
func musicParseLyricsPage(data map[string]any) string {
	var lyrics string

	walkJSON(data, func(m map[string]any) {
		if lyrics != "" {
			return
		}
		if r, ok := m["musicDescriptionShelfRenderer"].(map[string]any); ok {
			lyrics = extractText(r["description"])
		}
	})

	if lyrics == "" {
		walkJSON(data, func(m map[string]any) {
			if lyrics != "" {
				return
			}
			if r, ok := m["timedLyricsModel"].(map[string]any); ok {
				if ld, ok := r["lyricsData"].(map[string]any); ok {
					if lines, ok := ld["lines"].([]any); ok {
						var parts []string
						for _, line := range lines {
							if lm, ok := line.(map[string]any); ok {
								if txt := stringValue(lm["lyricLine"]); txt != "" {
									parts = append(parts, txt)
								}
							}
						}
						lyrics = strings.Join(parts, "\n")
					}
				}
			}
		})
	}

	return lyrics
}

// --- Music-local helpers ---

// musicExtractFlexColumns extracts text from flexColumns of a musicResponsiveListItemRenderer.
func musicExtractFlexColumns(r map[string]any) []string {
	var cols []string
	if fcs, ok := r["flexColumns"].([]any); ok {
		for _, fc := range fcs {
			if fcm, ok := fc.(map[string]any); ok {
				if renderer, ok := fcm["musicResponsiveListItemFlexColumnRenderer"].(map[string]any); ok {
					cols = append(cols, extractText(renderer["text"]))
				}
			}
		}
	}
	return cols
}

// musicExtractFixedColumns extracts text from fixedColumns of a musicResponsiveListItemRenderer.
func musicExtractFixedColumns(r map[string]any) []string {
	var cols []string
	if fcs, ok := r["fixedColumns"].([]any); ok {
		for _, fc := range fcs {
			if fcm, ok := fc.(map[string]any); ok {
				if renderer, ok := fcm["musicResponsiveListItemFixedColumnRenderer"].(map[string]any); ok {
					cols = append(cols, extractText(renderer["text"]))
				}
			}
		}
	}
	return cols
}

// musicExtractRunTexts collects the text runs from a runs-style field.
func musicExtractRunTexts(v any) []string {
	if m, ok := v.(map[string]any); ok {
		if runs, ok := m["runs"].([]any); ok {
			var parts []string
			for _, item := range runs {
				if rm, ok := item.(map[string]any); ok {
					if txt := stringValue(rm["text"]); txt != "" {
						parts = append(parts, txt)
					}
				}
			}
			return parts
		}
	}
	return nil
}

// musicExtractThumbnail extracts the best thumbnail URL from a renderer map.
func musicExtractThumbnail(r map[string]any) string {
	if thumb, ok := r["thumbnail"].(map[string]any); ok {
		if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
			if t, ok := tvm["thumbnail"].(map[string]any); ok {
				return bestThumbnail(t["thumbnails"])
			}
		}
	}
	if thumb, ok := r["thumbnailRenderer"].(map[string]any); ok {
		if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
			if t, ok := tvm["thumbnail"].(map[string]any); ok {
				return bestThumbnail(t["thumbnails"])
			}
		}
	}
	return ""
}

// musicExtractContinuation pulls a continuation token from a response.
func musicExtractContinuation(root any) string {
	var token string
	walkJSON(root, func(m map[string]any) {
		if token != "" {
			return
		}
		if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
			if ep, ok := cir["continuationEndpoint"].(map[string]any); ok {
				if cmd, ok := ep["continuationCommand"].(map[string]any); ok {
					if t := stringValue(cmd["token"]); t != "" {
						token = t
					}
				}
			}
		}
		if ncd, ok := m["nextContinuationData"].(map[string]any); ok {
			if t := stringValue(ncd["continuation"]); t != "" && token == "" {
				token = t
			}
		}
	})
	return token
}

// musicCarouselTitle extracts the title from a musicCarouselShelfRenderer.
func musicCarouselTitle(r map[string]any) string {
	if header, ok := r["header"].(map[string]any); ok {
		if basic, ok := header["musicCarouselShelfBasicHeaderRenderer"].(map[string]any); ok {
			return extractText(basic["title"])
		}
		return extractText(header["title"])
	}
	return ""
}

// musicParseTwoRowSong parses a musicTwoRowItemRenderer into a Song.
func musicParseTwoRowSong(r map[string]any) Song {
	s := Song{FetchedAt: time.Now()}
	s.Title = extractText(r["title"])
	if sub, ok := r["subtitle"]; ok {
		s.ArtistName = extractText(sub)
	}

	walkJSON(r, func(m map[string]any) {
		if s.VideoID != "" {
			return
		}
		if wep, ok := m["watchEndpoint"].(map[string]any); ok {
			if v := stringValue(wep["videoId"]); v != "" {
				s.VideoID = v
			}
		}
	})
	if s.VideoID != "" {
		s.URL = musicBaseURL + "/watch?v=" + s.VideoID
	}

	if thumb, ok := r["thumbnailRenderer"].(map[string]any); ok {
		if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
			if t, ok := tvm["thumbnail"].(map[string]any); ok {
				s.ThumbnailURL = bestThumbnail(t["thumbnails"])
			}
		}
	}
	return s
}

// musicParseTwoRowAlbum parses a musicTwoRowItemRenderer into an Album.
func musicParseTwoRowAlbum(r map[string]any) Album {
	alb := Album{FetchedAt: time.Now()}
	alb.Title = extractText(r["title"])

	if sub, ok := r["subtitle"]; ok {
		for _, p := range musicExtractRunTexts(sub) {
			p = strings.TrimSpace(p)
			pl := strings.ToLower(p)
			switch {
			case pl == "album" || pl == "single" || pl == "ep":
				if alb.AlbumType == "" {
					alb.AlbumType = p
				}
			case musicIsYear(p):
				if alb.Year == "" {
					alb.Year = p
				}
			case p != "" && p != "•" && p != "·" && alb.ArtistName == "":
				alb.ArtistName = p
			}
		}
	}

	if nav, ok := r["navigationEndpoint"].(map[string]any); ok {
		if be, ok := nav["browseEndpoint"].(map[string]any); ok {
			alb.AlbumID = stringValue(be["browseId"])
		}
	}
	if alb.AlbumID != "" {
		alb.URL = musicBaseURL + "/browse/" + alb.AlbumID
	}

	walkJSON(r, func(m map[string]any) {
		if alb.ArtistID != "" {
			return
		}
		if ep, ok := m["browseEndpoint"].(map[string]any); ok {
			bid := stringValue(ep["browseId"])
			if strings.HasPrefix(bid, "UC") {
				alb.ArtistID = bid
			}
		}
	})

	if thumb, ok := r["thumbnailRenderer"].(map[string]any); ok {
		if tvm, ok := thumb["musicThumbnailRenderer"].(map[string]any); ok {
			if t, ok := tvm["thumbnail"].(map[string]any); ok {
				alb.ThumbnailURL = bestThumbnail(t["thumbnails"])
			}
		}
	}
	return alb
}

// musicPageType returns the pageType string from a browseEndpointContextMusicConfig.
func musicPageType(ep map[string]any) string {
	if cfg, ok := ep["browseEndpointContextSupportedConfigs"].(map[string]any); ok {
		if mc, ok := cfg["browseEndpointContextMusicConfig"].(map[string]any); ok {
			return stringValue(mc["pageType"])
		}
	}
	return ""
}

// musicSplitDot splits on the Unicode bullet separator used by YTMusic subtitle runs.
func musicSplitDot(s string) []string {
	if parts := strings.Split(s, " • "); len(parts) > 1 {
		return parts
	}
	if parts := strings.Split(s, " · "); len(parts) > 1 {
		return parts
	}
	return strings.Split(s, " ·")
}

// musicLooksLikePlays reports whether s looks like a play-count string.
func musicLooksLikePlays(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(lower, "play") || strings.HasSuffix(lower, " plays")
}

// musicIsYear reports whether s looks like a 4-digit year.
func musicIsYear(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// padTwo zero-pads n to two digits.
func padTwo(n int) string {
	return fmt.Sprintf("%02d", n)
}

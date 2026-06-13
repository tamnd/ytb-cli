package youtube

import (
	"net/url"
	"strings"
)

// NormalizeVideoURL converts a video ID, youtu.be URL, shorts URL, or full watch URL
// to a canonical https://www.youtube.com/watch?v=ID form.
func NormalizeVideoURL(input string) string {
	if strings.Contains(input, "://") {
		if u, err := url.Parse(input); err == nil {
			if id := u.Query().Get("v"); id != "" {
				return BaseURL + "/watch?v=" + id
			}
			if strings.Contains(u.Host, "youtu.be") {
				id := strings.Trim(strings.TrimPrefix(u.Path, "/"), " ")
				if id != "" {
					return BaseURL + "/watch?v=" + id
				}
			}
			if parts := strings.Split(strings.Trim(u.Path, "/"), "/"); len(parts) >= 2 && parts[0] == "shorts" {
				return BaseURL + "/watch?v=" + parts[1]
			}
		}
		return input
	}
	return BaseURL + "/watch?v=" + input
}

// NormalizePlaylistURL converts a playlist ID or URL to a canonical
// https://www.youtube.com/playlist?list=ID form.
func NormalizePlaylistURL(input string) string {
	if strings.Contains(input, "://") {
		if u, err := url.Parse(input); err == nil {
			if id := u.Query().Get("list"); id != "" {
				return BaseURL + "/playlist?list=" + id
			}
		}
		return input
	}
	return BaseURL + "/playlist?list=" + input
}

// NormalizeChannelURL converts a channel ID, @handle, vanity name, or URL to
// a canonical https://www.youtube.com/... form that ends with /videos.
func NormalizeChannelURL(input string) string {
	if strings.Contains(input, "://") {
		u := strings.TrimSuffix(input, "/")
		if strings.Contains(u, "/videos") {
			return u
		}
		return u + "/videos"
	}
	if strings.HasPrefix(input, "@") {
		return BaseURL + "/" + input + "/videos"
	}
	if strings.HasPrefix(input, "UC") {
		return BaseURL + "/channel/" + input + "/videos"
	}
	return BaseURL + "/" + input + "/videos"
}

// NormalizeChannelID strips URL prefix and path suffix to return a bare channel
// ID (UC...) or handle (without @).
func NormalizeChannelID(input string) string {
	input = strings.TrimSpace(input)
	for _, prefix := range []string{
		"https://www.youtube.com/", "http://www.youtube.com/",
		"https://youtube.com/", "http://youtube.com/",
	} {
		if strings.HasPrefix(input, prefix) {
			input = strings.TrimPrefix(input, prefix)
			break
		}
	}
	if i := strings.Index(input, "/"); i >= 0 {
		input = input[:i]
	}
	input = strings.TrimPrefix(input, "@")
	return input
}

// ExtractVideoID extracts the video ID from a URL or returns the bare ID as-is.
func ExtractVideoID(input string) string {
	u := NormalizeVideoURL(input)
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("v")
}

// ExtractPlaylistID extracts the playlist ID from a URL or recognises bare IDs.
func ExtractPlaylistID(input string) string {
	if strings.Contains(input, "://") {
		if u, err := url.Parse(input); err == nil {
			if id := u.Query().Get("list"); id != "" {
				return id
			}
		}
		return ""
	}
	for _, pfx := range []string{"PL", "RD", "UU", "FL", "LL", "WL"} {
		if strings.HasPrefix(input, pfx) && len(input) > 10 {
			return input
		}
	}
	return ""
}

// extractPlaylistID is the package-private alias used by parsers.
func extractPlaylistID(input string) string {
	u := NormalizePlaylistURL(input)
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("list")
}

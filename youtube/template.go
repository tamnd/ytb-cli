package youtube

import (
	"regexp"
	"strconv"
	"strings"
)

// OutputFields supplies values for an output template. It mirrors the subset of
// yt-dlp's --output fields that the native engine can fill.
type OutputFields struct {
	ID            string
	Title         string
	Author        string // channel/uploader name
	ChannelID     string
	PlaylistTitle string
	PlaylistIndex int
	Ext           string
	Resolution    string
	Duration      int
}

var outputFieldRe = regexp.MustCompile(`%\(([a-zA-Z_]+)\)s`)

// RenderOutputTemplate expands a yt-dlp-style template such as
// "%(title)s [%(id)s].%(ext)s". Unknown fields expand to "NA". The result is
// sanitized component-by-component so path separators in titles cannot escape
// the intended directory.
func RenderOutputTemplate(tmpl string, f OutputFields) string {
	if tmpl == "" {
		tmpl = "%(title)s [%(id)s].%(ext)s"
	}
	expanded := outputFieldRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		key := outputFieldRe.FindStringSubmatch(m)[1]
		return sanitizeComponent(outputFieldValue(f, key))
	})
	// Sanitize each path component but keep the separators the template author
	// wrote intentionally.
	parts := strings.Split(expanded, "/")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return strings.Join(parts, "/")
}

func outputFieldValue(f OutputFields, key string) string {
	switch key {
	case "id":
		return f.ID
	case "title":
		return f.Title
	case "uploader", "channel", "author":
		return f.Author
	case "channel_id", "uploader_id":
		return f.ChannelID
	case "playlist", "playlist_title":
		return f.PlaylistTitle
	case "playlist_index":
		if f.PlaylistIndex > 0 {
			return strconv.Itoa(f.PlaylistIndex)
		}
		return ""
	case "ext":
		return f.Ext
	case "resolution":
		return f.Resolution
	case "duration":
		if f.Duration > 0 {
			return strconv.Itoa(f.Duration)
		}
		return ""
	default:
		return "NA"
	}
}

var unsafeFilenameRe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// sanitizeComponent strips characters that are illegal in filenames on common
// platforms, collapsing them to spaces, so a single template field stays within
// one path component.
func sanitizeComponent(s string) string {
	s = unsafeFilenameRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.Trim(s, " .")
}

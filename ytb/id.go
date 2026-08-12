package ytb

import (
	"context"

	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/ytb-cli/pkg/ytid"
)

// id.go is the record form of pkg/ytid.
//
// The classification itself lives in pkg/ytid, which reaches nothing and is
// importable on its own, because turning a channel id into five playlist ids is
// useful to anybody and hiding it behind this package's client would buy nothing.
// What is here is the record a surface can print, and the URL for the ids that
// have one.

// IDInfo is what an id says about itself before any request goes out.
type IDInfo struct {
	ID   string `json:"id" kit:"id" table:"id"`
	Kind string `json:"kind" table:"kind"`
	// Input is the string as it was given, which is worth keeping when a URL was
	// reduced to the id inside it.
	Input string `json:"input" table:"-"`
	// URL is the live page, and is empty when there is not one to offer. A mix has
	// no URL here on purpose: browsing one answers with a refusal, so handing back
	// a link would be handing back something that does not work.
	URL string `json:"url,omitempty" table:"url"`
	// ChannelID is filled when the id names a channel or derives one, so a
	// UU-family playlist id names a channel nobody fetched.
	ChannelID string `json:"channel_id,omitempty" table:"-"`
	// ParentID is a reply's comment, read off the dot.
	ParentID string `json:"parent_id,omitempty" table:"-"`
	// The five playlists a channel id derives, and the browse form of the first.
	//
	// These are columns rather than json-only fields, which makes the table wide for
	// a channel id and leaves eight empty cells for every other kind. That is the
	// trade the renderer offers: the readable vertical view, `-o list`, draws from the
	// same column set as the grid, so a field with table:"-" is missing from both. The
	// vertical block is the point of this command, and an empty cell for a video is a
	// true statement about a video, so the columns stay and `-c` narrows them for
	// anybody who wants a grid.
	Uploads string `json:"uploads,omitempty" table:"uploads"`
	Videos  string `json:"videos,omitempty" table:"videos"`
	Shorts  string `json:"shorts,omitempty" table:"shorts"`
	Streams string `json:"streams,omitempty" table:"streams"`
	Popular string `json:"popular,omitempty" table:"popular"`
	// Browse is the uploads playlist with VL in front. It is named for the wire
	// because that is the only place it belongs.
	Browse string `json:"browse,omitempty" table:"browse"`
	Feed   string `json:"feed,omitempty" table:"feed"`
	// NeedsRequest is set on a handle and the two legacy name forms, which name a
	// channel only after navigation/resolve_url has answered.
	NeedsRequest bool `json:"needs_request" table:"-"`
	// Unviewable is why there is nothing to read, and empty when there is. It is not
	// a column of its own because only one kind sets it, a mix, and a column that is
	// blank for the other nineteen kinds earns nothing. The note column carries its
	// sentence instead, so a mix still explains itself on a terminal.
	Unviewable string `json:"unviewable,omitempty" table:"-"`
	// Note is the one line worth reading about this id. It is the last column because
	// it is the only one that is a sentence rather than an id.
	Note string `json:"note,omitempty" table:"note,truncate"`
}

// ClassifyID reads any id, reference or URL and returns the record form, or nil
// when the input matches no shape.
func ClassifyID(input string) *IDInfo {
	info := ytid.Classify(input)
	if info.Kind == ytid.Unknown {
		return nil
	}
	out := &IDInfo{
		ID:           info.ID,
		Kind:         string(info.Kind),
		Input:        info.Input,
		ChannelID:    info.ChannelID,
		ParentID:     info.ParentID,
		NeedsRequest: info.NeedsRequest,
		Unviewable:   info.Unviewable,
		Note:         info.Note,
		URL:          idURL(info),
	}
	if out.Note == "" {
		out.Note = info.Unviewable
	}
	if pl := info.Playlists; pl != nil {
		out.Uploads, out.Videos, out.Shorts = pl.Uploads, pl.Videos, pl.Shorts
		out.Streams, out.Popular = pl.Streams, pl.Popular
		out.Browse, out.Feed = pl.Browse, pl.Feed
	}
	return out
}

// idURL is the live page for an id, or "" when there is not one. An id that
// refuses when read gets no URL, and neither does one that names a part of a page
// rather than a page: an itag is a format, a feed is a browse destination, and a
// comment id has no address of its own that works without the video.
func idURL(info ytid.Info) string {
	if info.Unviewable != "" {
		return ""
	}
	switch info.Kind {
	case ytid.Video:
		return BaseURL + "/watch?v=" + info.ID
	case ytid.Channel:
		return BaseURL + "/channel/" + info.ID
	case ytid.Handle:
		return BaseURL + "/" + info.ID
	case ytid.LegacyUser, ytid.LegacyCustom:
		return BaseURL + info.ID
	case ytid.Playlist, ytid.Uploads, ytid.Videos, ytid.Shorts, ytid.Streams, ytid.Popular, ytid.Album:
		return BaseURL + "/playlist?list=" + info.ID
	case ytid.MusicAlbum, ytid.MusicArtist:
		return MusicBaseURL + "/browse/" + info.ID
	case ytid.Hashtag:
		return BaseURL + "/hashtag/" + info.ID
	}
	return ""
}

type idRef struct {
	Ref string `kit:"arg" help:"any id, @handle, or URL"`
}

func getID(ctx context.Context, in idRef, emit func(*IDInfo) error) error {
	info := ClassifyID(in.Ref)
	if info == nil {
		return errs.Usage("%q matches no YouTube id shape", in.Ref)
	}
	return emit(info)
}

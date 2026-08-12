package ytb

import (
	"time"

	"github.com/tamnd/ytb-cli/pkg/graph"
	"github.com/tamnd/ytb-cli/pkg/rdf"
)

// rdf.go turns a record's fields into RDF statements. Spec 3005 doc 04 section 5.
//
// edges.go turns a record into claims, which are edges between nodes. This file
// is the other half: the literals. A title, a duration and a view count are not
// edges and they are most of what a person wants out of an export, so the
// writer takes both lists and merges them.
//
// The terms come off YouTube's own markup wherever it speaks. Where the mapping
// says inferred in the spec's table it is spelled the way x-cli and
// facebook-cli already spell it, so a store from all three tools joins.

// VideoStatements is what a Video record asserts about itself.
func VideoStatements(v Video) []rdf.Statement {
	if v.VideoID == "" {
		return nil
	}
	me := string(graph.VideoURI(v.VideoID))
	prov := v.Prov()
	src := []rdf.Source{{URL: prov.Source, Client: prov.Client}}
	var out []rdf.Statement
	add := func(pred string, o rdf.Object) {
		out = append(out, rdf.Statement{Subject: me, Predicate: pred, Object: o, Sources: src})
	}
	text := func(pred, val string) {
		if val != "" {
			add(pred, rdf.StringObject(val))
		}
	}

	add(rdf.NSRDF+"type", rdf.IRIObject(rdf.NSSchema+"VideoObject"))
	text(rdf.NSSchema+"identifier", v.VideoID)
	text(rdf.NSSchema+"name", v.Title)
	text(rdf.NSSchema+"description", v.Description)
	text(rdf.NSSchema+"genre", v.Category)
	if v.URL != "" {
		add(rdf.NSSchema+"url", rdf.IRIObject(v.URL))
	}
	if v.EmbedURL != "" {
		add(rdf.NSSchema+"embedUrl", rdf.IRIObject(v.EmbedURL))
	}
	if d := rdf.ISODuration(v.DurationSeconds); d != "" {
		text(rdf.NSSchema+"duration", d)
	}
	if !v.PublishedAt.IsZero() {
		add(rdf.NSSchema+"datePublished", rdf.DateTimeObject(v.PublishedAt.UTC().Format(time.RFC3339)))
	}
	if !v.UploadDate.IsZero() {
		add(rdf.NSSchema+"uploadDate", rdf.DateTimeObject(v.UploadDate.UTC().Format(time.RFC3339)))
	}
	for _, k := range v.Keywords {
		text(rdf.NSSchema+"keywords", k)
	}
	for _, c := range v.AvailableCountries {
		text(rdf.NSSchema+"regionsAllowed", c)
	}
	if v.IsFamilySafe != nil {
		add(rdf.NSSchema+"isFamilyFriendly", rdf.BoolObject(*v.IsFamilySafe))
	}
	if v.ThumbnailURL != "" {
		add(rdf.NSSchema+"thumbnailUrl", rdf.IRIObject(v.ThumbnailURL))
	}
	// The counts are InteractionCounter nodes because that is how the page writes
	// them, and each one gets a URI of its own rather than a blank node: a blank
	// node is renamed on every run and this output has to be diffable.
	out = append(out, counterStatements(me, "WatchAction", v.ViewCount, src)...)
	out = append(out, counterStatements(me, "LikeAction", v.LikeCount, src)...)
	return out
}

// counterStatements writes one InteractionCounter. A zero count is not written:
// zero views and a view count nobody read are the same absent field on the
// record and asserting the first would be inventing an answer.
func counterStatements(subject, action string, count int64, src []rdf.Source) []rdf.Statement {
	if count <= 0 {
		return nil
	}
	node := subject + "#interaction/" + action
	return []rdf.Statement{
		{Subject: subject, Predicate: rdf.NSSchema + "interactionStatistic", Object: rdf.IRIObject(node), Sources: src},
		{Subject: node, Predicate: rdf.NSRDF + "type", Object: rdf.IRIObject(rdf.NSSchema + "InteractionCounter")},
		{Subject: node, Predicate: rdf.NSSchema + "interactionType", Object: rdf.IRIObject(rdf.NSSchema + action), Sources: src},
		{Subject: node, Predicate: rdf.NSSchema + "userInteractionCount", Object: rdf.IntObject(count), Sources: src},
	}
}

// ChannelStatements is what a Channel record asserts about itself.
//
// A channel is a schema:Person because that is what the channel page's own
// ld+json calls it, on a channel run by a company as much as one run by a
// person. The site is not being asked to be consistent, it is being quoted.
func ChannelStatements(c Channel) []rdf.Statement {
	me := string(graph.ChannelURI(c.ChannelID))
	if me == "" {
		return nil
	}
	prov := c.Prov()
	src := []rdf.Source{{URL: prov.Source, Client: prov.Client}}
	var out []rdf.Statement
	add := func(pred string, o rdf.Object) {
		out = append(out, rdf.Statement{Subject: me, Predicate: pred, Object: o, Sources: src})
	}
	text := func(pred, val string) {
		if val != "" {
			add(pred, rdf.StringObject(val))
		}
	}

	add(rdf.NSRDF+"type", rdf.IRIObject(rdf.NSSchema+"Person"))
	text(rdf.NSSchema+"identifier", c.ChannelID)
	text(rdf.NSSchema+"name", c.Title)
	text(rdf.NSSchema+"description", c.Description)
	// The handle is alternateName because that is the term the channel page's
	// ld+json uses for it, and because it is a name rather than an identity: a
	// channel can change it and two spellings reach one channel.
	text(rdf.NSSchema+"alternateName", c.Handle)
	if c.CanonicalURL != "" {
		add(rdf.NSSchema+"url", rdf.IRIObject(c.CanonicalURL))
	}
	for _, l := range c.Links {
		if l.URL != "" {
			// sameAs, which is what the page's ld+json calls these: they are the
			// channel elsewhere rather than something it cited. The links_to claim
			// writes the same link a second time as schema:citation, and that is not
			// a duplicate worth removing: the claim points at yt://external/<sha256>,
			// which is the node other records join on, and this keeps the address
			// itself, which a hash does not give back.
			add(rdf.NSSchema+"sameAs", rdf.IRIObject(l.URL))
		}
	}
	if !c.JoinedAt.IsZero() {
		add(rdf.NSSchema+"dateCreated", rdf.DateTimeObject(c.JoinedAt.UTC().Format(time.RFC3339)))
	}
	out = append(out, counterStatements(me, "FollowAction", c.SubscriberCount, src)...)
	out = append(out, counterStatements(me, "WatchAction", c.ViewCount, src)...)
	return out
}

// PlaylistStatements is what a Playlist record asserts about itself.
func PlaylistStatements(p Playlist) []rdf.Statement {
	if p.PlaylistID == "" {
		return nil
	}
	me := string(graph.PlaylistURI(p.PlaylistID))
	prov := p.Prov()
	src := []rdf.Source{{URL: prov.Source, Client: prov.Client}}
	var out []rdf.Statement
	add := func(pred string, o rdf.Object) {
		out = append(out, rdf.Statement{Subject: me, Predicate: pred, Object: o, Sources: src})
	}
	add(rdf.NSRDF+"type", rdf.IRIObject(rdf.NSSchema+"ItemList"))
	// An ordered list, and saying so is the difference between a playlist and a
	// bag of videos. The order itself rides on the contains claims.
	add(rdf.NSSchema+"itemListOrder", rdf.IRIObject(rdf.NSSchema+"ItemListOrderAscending"))
	if p.Title != "" {
		add(rdf.NSSchema+"name", rdf.StringObject(p.Title))
	}
	if p.Description != "" {
		add(rdf.NSSchema+"description", rdf.StringObject(p.Description))
	}
	if p.VideoCount > 0 {
		add(rdf.NSSchema+"numberOfItems", rdf.IntObject(int64(p.VideoCount)))
	}
	return out
}

// MicrodataStatements is the page's own VideoObject as triples, which is the
// other side of ytb rdf --check.
//
// Two things are normalised on the way in and both cost an afternoon if they
// are not. The author url is plain http on a page served over TLS, and it names
// a handle rather than a channel id, so the subject of the author claim is the
// URL the page gave and the comparison normalises the scheme.
func MicrodataStatements(md *Microdata, videoID string) []rdf.Statement {
	if md == nil || videoID == "" {
		return nil
	}
	me := string(graph.VideoURI(videoID))
	src := []rdf.Source{{URL: md.ItemID}}
	var out []rdf.Statement
	add := func(pred string, o rdf.Object) {
		out = append(out, rdf.Statement{Subject: me, Predicate: pred, Object: o, Sources: src})
	}
	text := func(pred, val string) {
		if val != "" {
			add(pred, rdf.StringObject(val))
		}
	}

	if md.ItemType != "" {
		add(rdf.NSRDF+"type", rdf.IRIObject(httpsScheme(md.ItemType)))
	}
	text(rdf.NSSchema+"identifier", md.Identifier)
	text(rdf.NSSchema+"name", md.Name)
	text(rdf.NSSchema+"description", md.Description)
	text(rdf.NSSchema+"genre", md.Genre)
	text(rdf.NSSchema+"duration", md.Duration)
	if md.URL != "" {
		add(rdf.NSSchema+"url", rdf.IRIObject(md.URL))
	}
	if md.EmbedURL != "" {
		add(rdf.NSSchema+"embedUrl", rdf.IRIObject(md.EmbedURL))
	}
	if !md.DatePublished.IsZero() {
		add(rdf.NSSchema+"datePublished", rdf.DateTimeObject(md.DatePublished.UTC().Format(time.RFC3339)))
	}
	if !md.UploadDate.IsZero() {
		add(rdf.NSSchema+"uploadDate", rdf.DateTimeObject(md.UploadDate.UTC().Format(time.RFC3339)))
	}
	for _, k := range md.Keywords {
		text(rdf.NSSchema+"keywords", k)
	}
	for _, c := range md.RegionsAllowed {
		text(rdf.NSSchema+"regionsAllowed", c)
	}
	if md.IsFamilyFriendly != nil {
		add(rdf.NSSchema+"isFamilyFriendly", rdf.BoolObject(*md.IsFamilyFriendly))
	}
	if md.ThumbnailURL != "" {
		add(rdf.NSSchema+"thumbnailUrl", rdf.IRIObject(md.ThumbnailURL))
	}
	if md.AuthorURL != "" {
		add(rdf.NSSchema+"author", rdf.IRIObject(md.AuthorURL))
	}
	out = append(out, counterStatements(me, "WatchAction", md.ViewCount, src)...)
	out = append(out, counterStatements(me, "LikeAction", md.LikeCount, src)...)
	return out
}

// VideoAliases are the addresses the site used for things this tool names by
// URI, taken off the same read.
//
// The author on a watch page is the case this exists for. The page states it as
// http://www.youtube.com/@RickAstleyYT: a handle, on a page served over TLS,
// where ytb says yt://channel/UCuAXFkgsw1L7xaCfnd5JJOw. Those are the same
// answer spelled differently, and the only reason this tool can say so is that
// the same response named both, so the alias is evidence rather than a guess.
func VideoAliases(v Video) map[string]string {
	out := map[string]string{}
	uri := string(graph.ChannelURI(v.ChannelID))
	if uri == "" {
		return out
	}
	for _, url := range []string{v.ChannelURL, BaseURL + "/channel/" + v.ChannelID} {
		if url != "" {
			out[url] = uri
		}
	}
	if v.ChannelHandle != "" {
		out[BaseURL+"/"+v.ChannelHandle] = uri
	}
	return out
}

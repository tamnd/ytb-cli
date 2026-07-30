package ytid

import "testing"

// Every id in this file is a real one, read off a live response while writing
// milestone 4, except the comment and reply ids. Those two are the shape rather
// than a capture: YouTube answers this address with Restricted Mode and hides
// comments, so there is no real comment id to be had from here. The rule they
// exercise is a delimiter rather than an encoding, and it holds for any
// well-formed pair.
const (
	realVideo   = "dQw4w9WgXcQ"
	realChannel = "UCuAXFkgsw1L7xaCfnd5JJOw"
	realHandle  = "@RickAstleyYT"
	// Read off ytb playlists @RickAstleyYT.
	realPlaylist = "PLlaN88a7y2_qHDbY9eQbuNTAuEJUSEeuu"
	// Read off ytb music album MPREb_dcYZhAh5urI.
	realAlbum      = "OLAK5uy_nmDUsWOMoEcz0SsVqUwir0oxu-k1oUyXE"
	realMusicAlbum = "MPREb_dcYZhAh5urI"
	// Rick Astley's music channel, which is a different channel from his own.
	realArtistChannel = "UCwZEU0wAwIyZb4x5G_KJp2w"
)

// TestClassifyEveryShape is the test per shape the milestone asks for. One table,
// one row per row of doc 04 section 1.
func TestClassifyEveryShape(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
		id   string
	}{
		{realVideo, Video, realVideo},
		{realChannel, Channel, realChannel},
		{realHandle, Handle, realHandle},
		{"/user/RickAstleyVEVO", LegacyUser, "/user/RickAstleyVEVO"},
		{"/c/RickAstley", LegacyCustom, "/c/RickAstley"},
		{realPlaylist, Playlist, realPlaylist},
		{"UUuAXFkgsw1L7xaCfnd5JJOw", Uploads, "UUuAXFkgsw1L7xaCfnd5JJOw"},
		{"UULFuAXFkgsw1L7xaCfnd5JJOw", Videos, "UULFuAXFkgsw1L7xaCfnd5JJOw"},
		{"UUSHuAXFkgsw1L7xaCfnd5JJOw", Shorts, "UUSHuAXFkgsw1L7xaCfnd5JJOw"},
		{"UULVuAXFkgsw1L7xaCfnd5JJOw", Streams, "UULVuAXFkgsw1L7xaCfnd5JJOw"},
		{"UULPuAXFkgsw1L7xaCfnd5JJOw", Popular, "UULPuAXFkgsw1L7xaCfnd5JJOw"},
		{realAlbum, Album, realAlbum},
		{"RD" + realVideo, Mix, "RD" + realVideo},
		{"RDCMUCuAXFkgsw1L7xaCfnd5JJOw", Mix, "RDCMUCuAXFkgsw1L7xaCfnd5JJOw"},
		// VL is routing, so the kind is the playlist's and the id comes back bare.
		{"VLUUuAXFkgsw1L7xaCfnd5JJOw", Uploads, "UUuAXFkgsw1L7xaCfnd5JJOw"},
		{realMusicAlbum, MusicAlbum, realMusicAlbum},
		{"MPLA" + realArtistChannel, MusicArtist, "MPLA" + realArtistChannel},
		{"FEwhat_to_watch", Feed, "FEwhat_to_watch"},
		{"FEtrending", Feed, "FEtrending"},
		{"FEmusic_home", Feed, "FEmusic_home"},
		{"UgwOEVnBQ0FSYnJmVVo0AaABAg", Comment, "UgwOEVnBQ0FSYnJmVVo0AaABAg"},
		{"UgwOEVnBQ0FSYnJmVVo0AaABAg.9Xn3PqR2Abc", Reply, "UgwOEVnBQ0FSYnJmVVo0AaABAg.9Xn3PqR2Abc"},
		{"140", Itag, "140"},
		{"251", Itag, "251"},
		{"18", Itag, "18"},
		{"LL", Playlist, "LL"},
		{"WL", Playlist, "WL"},
	}
	for _, c := range cases {
		got := Classify(c.in)
		if got.Kind != c.want {
			t.Errorf("Classify(%q).Kind = %q, want %q", c.in, got.Kind, c.want)
		}
		if got.ID != c.id {
			t.Errorf("Classify(%q).ID = %q, want %q", c.in, got.ID, c.id)
		}
	}
}

// TestClassifyRefusesToGuess asserts an unrecognised string comes back Unknown.
// The old behaviour in the youtube package was to take anything left over for a
// video id, which turns a typo into a 404 from the far end and reads as a video
// that was deleted.
func TestClassifyRefusesToGuess(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"not an id",
		"dQw4w9WgXc",   // ten characters, one short of a video id
		"dQw4w9WgXcQQ", // twelve, one over
		"UCtooshort",   // a UC prefix with the wrong length
		"MPLAnotachannel",
		"@ab", // a handle is at least three characters after the @
	} {
		if got := Classify(in); got.Kind != Unknown {
			t.Errorf("Classify(%q) = %q, want Unknown", in, got.Kind)
		}
	}
}

// TestAVideoIDStartingWithAPrefixIsStillAVideo is the trap the length checks
// exist for. Video ids are random base64url, so one in a few thousand starts with
// RD or FE, and reading those as a mix or a feed would send an ordinary video to
// the wrong surface.
func TestAVideoIDStartingWithAPrefixIsStillAVideo(t *testing.T) {
	for _, in := range []string{"RDabc123xyz", "FEabcdefghi", "UUabc123xyz", "PLabc123xyz"} {
		if got := Classify(in); got.Kind != Video {
			t.Errorf("Classify(%q) = %q, want video: eleven characters is a video id", in, got.Kind)
		}
	}
}

// TestChannelDerivesItsFivePlaylists is the highest value thing in the package.
// All five ids below were browsed live: 435 uploads, 139 videos, 294 shorts, 2
// streams, 139 popular. The three kind-specific ones partition the uploads
// playlist exactly, 139 plus 294 plus 2 is 435.
func TestChannelDerivesItsFivePlaylists(t *testing.T) {
	pl, ok := PlaylistsFor(realChannel)
	if !ok {
		t.Fatalf("PlaylistsFor(%q) refused a real channel id", realChannel)
	}
	want := ChannelPlaylists{
		Uploads: "UUuAXFkgsw1L7xaCfnd5JJOw",
		Videos:  "UULFuAXFkgsw1L7xaCfnd5JJOw",
		Shorts:  "UUSHuAXFkgsw1L7xaCfnd5JJOw",
		Streams: "UULVuAXFkgsw1L7xaCfnd5JJOw",
		Popular: "UULPuAXFkgsw1L7xaCfnd5JJOw",
		Browse:  "VLUUuAXFkgsw1L7xaCfnd5JJOw",
		Feed:    "https://www.youtube.com/feeds/videos.xml?channel_id=" + realChannel,
	}
	if pl != want {
		t.Errorf("PlaylistsFor(%q) =\n%+v\nwant\n%+v", realChannel, pl, want)
	}
	// Classify hands the same block back, which is what ytb id prints.
	info := Classify(realChannel)
	if info.Playlists == nil || *info.Playlists != want {
		t.Errorf("Classify(%q) did not carry the derived playlists", realChannel)
	}
}

// TestPlaylistsForRefusesANonChannel asserts it does not build ids out of a
// string that was never a channel id, which would produce five ids that look
// right and browse to nothing.
func TestPlaylistsForRefusesANonChannel(t *testing.T) {
	for _, in := range []string{realVideo, realPlaylist, "UCshort", realHandle, ""} {
		if _, ok := PlaylistsFor(in); ok {
			t.Errorf("PlaylistsFor(%q) returned playlists for something that is not a channel id", in)
		}
	}
}

// TestEveryUUFamilyIDStripsBackToOneChannel is the reverse direction: a UU id
// found in a payload names a channel nobody fetched.
func TestEveryUUFamilyIDStripsBackToOneChannel(t *testing.T) {
	for _, in := range []string{
		"UUuAXFkgsw1L7xaCfnd5JJOw",
		"UULFuAXFkgsw1L7xaCfnd5JJOw",
		"UUSHuAXFkgsw1L7xaCfnd5JJOw",
		"UULVuAXFkgsw1L7xaCfnd5JJOw",
		"UULPuAXFkgsw1L7xaCfnd5JJOw",
		// The wire form strips too, so a browseId read out of a payload works.
		"VLUUuAXFkgsw1L7xaCfnd5JJOw",
	} {
		got, ok := ChannelFor(in)
		if !ok {
			t.Errorf("ChannelFor(%q) found no channel", in)
			continue
		}
		if got != realChannel {
			t.Errorf("ChannelFor(%q) = %q, want %q", in, got, realChannel)
		}
		if info := Classify(in); info.ChannelID != realChannel {
			t.Errorf("Classify(%q).ChannelID = %q, want %q", in, info.ChannelID, realChannel)
		}
	}
	// A playlist somebody made says nothing about a channel, and must not be made
	// to: PLlaN88a7y2_... has no channel in it.
	if got, ok := ChannelFor(realPlaylist); ok {
		t.Errorf("ChannelFor(%q) = %q, want no channel", realPlaylist, got)
	}
}

// TestReplyCarriesItsParent asserts the dot rule, which is the whole of comment
// threading with no request.
func TestReplyCarriesItsParent(t *testing.T) {
	const parent = "UgwOEVnBQ0FSYnJmVVo0AaABAg"
	const reply = parent + ".9Xn3PqR2Abc"
	got, ok := ParentOf(reply)
	if !ok {
		t.Fatalf("ParentOf(%q) found no parent", reply)
	}
	if got != parent {
		t.Errorf("ParentOf(%q) = %q, want %q", reply, got, parent)
	}
	if info := Classify(reply); info.Kind != Reply || info.ParentID != parent {
		t.Errorf("Classify(%q) = %q parent %q, want reply parent %q", reply, info.Kind, info.ParentID, parent)
	}
	// A top-level comment has no parent, and must not be given one.
	if got, ok := ParentOf(parent); ok {
		t.Errorf("ParentOf(%q) = %q, want no parent for a top-level comment", parent, got)
	}
	if _, ok := ParentOf("not.an.id"); ok {
		t.Error("ParentOf split a string that is not a comment id")
	}
}

// TestVLGoesOnTheWireAndNowhereElse asserts the routing prefix is added at the
// request and stripped everywhere else. A bare playlist id sent as a browseId is
// a 400 with a body that names no field, so this is worth pinning down.
func TestVLGoesOnTheWireAndNowhereElse(t *testing.T) {
	const bare = "UUuAXFkgsw1L7xaCfnd5JJOw"
	if got := WireID(bare); got != "VL"+bare {
		t.Errorf("WireID(%q) = %q, want VL in front", bare, got)
	}
	// Adding it twice is what a second caller does, and it must not happen.
	if got := WireID("VL" + bare); got != "VL"+bare {
		t.Errorf("WireID(%q) doubled the prefix: %q", "VL"+bare, got)
	}
	if got := WireID(""); got != "" {
		t.Errorf("WireID(\"\") = %q, want empty rather than a bare VL", got)
	}
	if got := StripWire("VL" + bare); got != bare {
		t.Errorf("StripWire(%q) = %q, want %q", "VL"+bare, got, bare)
	}
	if got := StripWire(bare); got != bare {
		t.Errorf("StripWire(%q) changed an id that had no prefix: %q", bare, got)
	}
	// The classified id never carries it, which is what keeps it out of a URI and
	// out of the store.
	if got := Classify("VL" + bare).ID; got != bare {
		t.Errorf("Classify(%q).ID = %q, want the prefix gone", "VL"+bare, got)
	}
}

// TestMixIsUnviewableRatherThanAURL asserts the mix answer. Browsing RD<video id>
// returns 200 with four keys and "This playlist type is unviewable.", so offering
// a URL for it would be offering something that refuses.
func TestMixIsUnviewableRatherThanAURL(t *testing.T) {
	for _, in := range []string{"RD" + realVideo, "RDCMUCuAXFkgsw1L7xaCfnd5JJOw", "RDMMoi27WMEfz3E"} {
		info := Classify(in)
		if info.Kind != Mix {
			t.Errorf("Classify(%q) = %q, want mix", in, info.Kind)
			continue
		}
		if info.Unviewable == "" {
			t.Errorf("Classify(%q) reports a mix with no reason it cannot be read", in)
		}
	}
	// Everything else that is viewable says nothing in that field.
	for _, in := range []string{realVideo, realChannel, realPlaylist, realAlbum} {
		if got := Classify(in).Unviewable; got != "" {
			t.Errorf("Classify(%q).Unviewable = %q, want empty", in, got)
		}
	}
}

// TestAliasesNeedARequest asserts the three shapes that name a channel only after
// navigation/resolve_url has answered, and that a real channel id does not.
func TestAliasesNeedARequest(t *testing.T) {
	for _, in := range []string{realHandle, "/user/RickAstleyVEVO", "/c/RickAstley"} {
		if !Classify(in).NeedsRequest {
			t.Errorf("Classify(%q) claims to name a channel without a request", in)
		}
		if got := Classify(in).ChannelID; got != "" {
			t.Errorf("Classify(%q).ChannelID = %q, want empty until something resolves it", in, got)
		}
	}
	if Classify(realChannel).NeedsRequest {
		t.Errorf("Classify(%q) wants a request for an id that is already a channel", realChannel)
	}
}

// TestClassifyURLs asserts the URL forms reduce to the id they carry. A watch
// link with a list on it is the one worth stating: the video wins, because that
// is what a person means by the link.
func TestClassifyURLs(t *testing.T) {
	cases := []struct {
		in   string
		want Kind
		id   string
	}{
		{"https://www.youtube.com/watch?v=" + realVideo, Video, realVideo},
		{"https://youtu.be/" + realVideo, Video, realVideo},
		{"https://www.youtube.com/shorts/" + realVideo, Video, realVideo},
		{"https://www.youtube.com/live/" + realVideo, Video, realVideo},
		{"https://www.youtube.com/embed/" + realVideo, Video, realVideo},
		{"youtube.com/watch?v=" + realVideo, Video, realVideo},
		{"https://www.youtube.com/watch?v=" + realVideo + "&list=" + realPlaylist, Video, realVideo},
		{"https://www.youtube.com/playlist?list=" + realPlaylist, Playlist, realPlaylist},
		{"https://www.youtube.com/channel/" + realChannel, Channel, realChannel},
		{"https://www.youtube.com/" + realHandle, Handle, realHandle},
		{"https://www.youtube.com/" + realHandle + "/videos", Handle, realHandle},
		{"https://www.youtube.com/user/RickAstleyVEVO", LegacyUser, "/user/RickAstleyVEVO"},
		{"https://www.youtube.com/c/RickAstley", LegacyCustom, "/c/RickAstley"},
		{"https://www.youtube.com/hashtag/rickroll", Hashtag, "rickroll"},
		{"https://music.youtube.com/browse/" + realMusicAlbum, MusicAlbum, realMusicAlbum},
		{"https://music.youtube.com/watch?v=" + realVideo, Video, realVideo},
	}
	for _, c := range cases {
		got := Classify(c.in)
		if got.Kind != c.want || got.ID != c.id {
			t.Errorf("Classify(%q) = %q %q, want %q %q", c.in, got.Kind, got.ID, c.want, c.id)
		}
	}
}

// TestIsPlaylistCoversEveryPlaylistShape asserts the question a request builder
// asks: does this need VL in front of it.
func TestIsPlaylistCoversEveryPlaylistShape(t *testing.T) {
	for _, in := range []string{
		realPlaylist,
		"UUuAXFkgsw1L7xaCfnd5JJOw",
		"UULFuAXFkgsw1L7xaCfnd5JJOw",
		"UUSHuAXFkgsw1L7xaCfnd5JJOw",
		"UULVuAXFkgsw1L7xaCfnd5JJOw",
		"UULPuAXFkgsw1L7xaCfnd5JJOw",
		realAlbum,
		"RD" + realVideo,
	} {
		if !IsPlaylist(in) {
			t.Errorf("IsPlaylist(%q) = false", in)
		}
	}
	for _, in := range []string{realVideo, realChannel, realHandle, realMusicAlbum, "FEtrending", "140"} {
		if IsPlaylist(in) {
			t.Errorf("IsPlaylist(%q) = true", in)
		}
	}
}

// TestIsChannelAndIsVideo pins the two shape questions the rest of the tool asks
// most often.
func TestIsChannelAndIsVideo(t *testing.T) {
	if !IsChannel(realChannel) || IsChannel(realVideo) || IsChannel("UC") {
		t.Error("IsChannel does not hold on the real ids")
	}
	if !IsVideo(realVideo) || IsVideo(realChannel) || IsVideo("") {
		t.Error("IsVideo does not hold on the real ids")
	}
}

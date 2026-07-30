package youtube

import (
	"strings"
	"testing"
)

// The fixture is @RickAstleyYT's real about panel, as the continuation returned
// it. Every assertion below is a value that came off the wire.
func TestParseChannelAboutRealPanel(t *testing.T) {
	about := ParseChannelAbout(loadFixture(t, "about_channel.json"))
	if about == nil {
		t.Fatal("no aboutChannelViewModel found")
	}
	if about.ChannelID != "UCuAXFkgsw1L7xaCfnd5JJOw" {
		t.Errorf("channel id = %q", about.ChannelID)
	}
	if about.Country != "United Kingdom" {
		t.Errorf("country = %q", about.Country)
	}
	if !strings.Contains(about.JoinedDateText, "Feb 1, 2015") {
		t.Errorf("joined = %q", about.JoinedDateText)
	}
	if about.SubscribersText == "" || about.ViewsText == "" || about.VideosText == "" {
		t.Errorf("counts = %q / %q / %q", about.SubscribersText, about.ViewsText, about.VideosText)
	}
	if about.DisplayURL != "www.youtube.com/@RickAstleyYT" {
		t.Errorf("display url = %q", about.DisplayURL)
	}
	if about.CanonicalURL == about.DisplayURL {
		t.Error("canonical and display urls should differ in scheme, so both are worth keeping")
	}
	if about.BusinessEmailText == "" {
		t.Error("business email prompt should be kept as text, not dropped")
	}
	if about.Labels.Description != "Description" || about.Labels.ArtistBio != "Biography" {
		t.Errorf("labels = %+v", about.Labels)
	}
}

// The one confusion this parser exists to prevent. The description is what the
// owner wrote and the bio is what a third party wrote, and merging them would put
// words in the owner's mouth.
func TestChannelAboutKeepsDescriptionAndBioApart(t *testing.T) {
	about := ParseChannelAbout(loadFixture(t, "about_channel.json"))
	if !strings.Contains(about.Description, "Raindrops") {
		t.Errorf("description should be the owner's own text, got %q", about.Description)
	}
	if !strings.Contains(about.ArtistBio, "Stock Aitken Waterman") {
		t.Errorf("artist bio should be the third party biography, got %q", truncate(about.ArtistBio, 80))
	}
	if about.Description == about.ArtistBio {
		t.Fatal("description and artist bio are the same value, so one overwrote the other")
	}
	if strings.Contains(about.Description, "Stock Aitken Waterman") {
		t.Error("the biography leaked into the description")
	}
}

// Every outbound link is wrapped in youtube.com/redirect with a redir_token that
// expires. Storing the wrapper stores a link that stops working.
func TestChannelAboutLinksAreUnwrapped(t *testing.T) {
	about := ParseChannelAbout(loadFixture(t, "about_channel.json"))
	if len(about.Links) == 0 {
		t.Fatal("no links read")
	}
	for _, l := range about.Links {
		if strings.Contains(l.URL, "/redirect?") || strings.Contains(l.URL, "redir_token") {
			t.Errorf("link %q still wrapped: %s", l.Title, l.URL)
		}
		if !strings.HasPrefix(l.URL, "http") {
			t.Errorf("link %q has no scheme: %s", l.Title, l.URL)
		}
		if l.Display == "" {
			t.Errorf("link %q has no display form", l.Title)
		}
	}
}

// The panel carries 18 fields. A nineteenth should fail the build rather than be
// dropped in silence, so every key is either mapped to a field or listed as
// ignored with a reason.
func TestChannelAboutReadsAllEighteenFields(t *testing.T) {
	resp := loadFixture(t, "about_channel.json")
	var vm map[string]any
	walkJSON(resp, func(m map[string]any) {
		if v, ok := m["aboutChannelViewModel"].(map[string]any); ok && vm == nil {
			vm = v
		}
	})
	if vm == nil {
		t.Fatal("no aboutChannelViewModel in the fixture")
	}
	if len(vm) != 18 {
		t.Errorf("the panel now has %d fields, not 18; update ChannelAbout and this count", len(vm))
	}
	for key := range vm {
		if _, ok := aboutViewModelFields[key]; !ok {
			t.Errorf("aboutChannelViewModel field %q is not read and not listed as ignored", key)
		}
	}
	for key := range aboutViewModelFields {
		if _, ok := vm[key]; !ok {
			t.Errorf("aboutViewModelFields lists %q, which the panel no longer carries", key)
		}
	}
}

func TestFindAboutTokenTakesTheEngagementPanel(t *testing.T) {
	if got := FindAboutToken(aboutPanelTree("TOK_ABOUT")); got != "TOK_ABOUT" {
		t.Fatalf("about token = %q, want TOK_ABOUT", got)
	}
}

// TestFindAboutTokenIgnoresAShelfPanel is the @Computerphile case.
//
// Its page carries five showEngagementPanelEndpoints. Two are the about panel,
// reached from the header, and three belong to a shelf on the featured tab titled
// "Brady Haran's other channels". Asking for showEngagementPanelEndpoint alone took
// the shelf, because /contents sorts before /header, and the about read came back
// with 25 KB of channel items and no aboutChannelViewModel. The panels are not
// labelled: each has an opaque tag, and the about panel's header title is the
// channel's own name, so where the token hangs is the only thing to go on.
func TestFindAboutTokenIgnoresAShelfPanel(t *testing.T) {
	page := aboutPanelTree("TOK_ABOUT")
	for k, v := range shelfPanelTree("TOK_SHELF") {
		page[k] = v
	}
	if got := FindAboutToken(page); got != "TOK_ABOUT" {
		t.Fatalf("about token = %q, want TOK_ABOUT: a shelf panel is an engagement panel too", got)
	}
}

func TestUnwrapRedirect(t *testing.T) {
	cases := map[string]string{
		"https://www.youtube.com/redirect?event=channel_description&redir_token=ABC&q=https%3A%2F%2Frickastley.lnk.to%2FRaindrops": "https://rickastley.lnk.to/Raindrops",
		"https://rickastley.co.uk/": "https://rickastley.co.uk/",
		"":                          "",
	}
	for in, want := range cases {
		if got := unwrapRedirect(in); got != want {
			t.Errorf("unwrapRedirect(%q) = %q, want %q", in, got, want)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

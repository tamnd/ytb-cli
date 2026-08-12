package ytb

import (
	"net/http"
	"net/url"
	"strings"
)

// locale.go pins the language of every read, in three places at once.
//
// This is not belt and braces for its own sake. A bare fetch of a YouTube page
// with only a User-Agent returns the page in the language of the requesting
// address: from this network that is Vietnamese, and the tab strip came back as
// "Trang chu / Video / Shorts / Phat truc tiep / Danh sach phat". A parser that
// matches a tab by its English title finds nothing on that page and reports an
// empty channel, which is the worst kind of wrong because it looks like an
// answer.
//
// Spec 3005 doc 01 section 1.3 measured three independent fixes: hl and gl on
// the URL, an Accept-Language header, and a PREF cookie. Any one of them is
// enough. We send all three, because they cost nothing and the failure they
// prevent is silent.
//
// The InnerTube context carries hl and gl as well, in clients.go. That is the
// fourth place, and it is the one that governs a POST with no URL query to hang
// them on.
//
// None of this makes a rendered string safe to compare against. It makes the
// language predictable, which is a different thing. Tabs are still matched by
// their params blob and counts are still read from their numeric fields.

// localise adds hl and gl to a youtube.com URL that does not already carry them.
// A URL that names its own hl is left alone: `ytb transcript --lang de` means it.
func (c *Client) localise(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if !isYouTubeHost(u.Host) {
		return raw
	}
	q := u.Query()
	if q.Get("hl") == "" && c.hl != "" {
		q.Set("hl", c.hl)
	}
	if q.Get("gl") == "" && c.gl != "" {
		q.Set("gl", c.gl)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// setLanguageHeaders sets Accept-Language and the two cookies every www request
// carries: PREF for the language and SOCS for consent.
//
// SOCS=CAI is the "reject non-essential" consent choice. Without a consent
// cookie some regions get a consent interstitial instead of the page, and the
// interstitial parses as a page with no data in it.
func (c *Client) setLanguageHeaders(req *http.Request) {
	hl := c.hl
	if hl == "" {
		hl = "en"
	}
	req.Header.Set("Accept-Language", acceptLanguage(hl))
	if !isYouTubeHost(req.URL.Host) {
		return
	}
	req.AddCookie(&http.Cookie{Name: "PREF", Value: "hl=" + hl + "&gl=" + c.gl})
	req.AddCookie(&http.Cookie{Name: "SOCS", Value: "CAI"})
	// CONSENT=YES+ is the older spelling. YouTube still honours it and some
	// edges appear to check only one of the two, so both go on.
	req.AddCookie(&http.Cookie{Name: "CONSENT", Value: "YES+"})
}

// acceptLanguage turns an hl code into a header value that also accepts English,
// so a page with no translation for hl still arrives in a language we parse
// rather than in the language of the address.
func acceptLanguage(hl string) string {
	if hl == "en" || strings.HasPrefix(hl, "en-") {
		return "en-US,en;q=0.9"
	}
	return hl + "," + hl + ";q=0.9,en;q=0.8"
}

// isYouTubeHost reports whether a host is one we may attach YouTube cookies and
// query parameters to. A googlevideo media URL is deliberately not one: it is
// signed, and adding a parameter to a signed URL invalidates it.
func isYouTubeHost(host string) bool {
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	switch host {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		return true
	}
	return strings.HasSuffix(host, ".youtube.com")
}

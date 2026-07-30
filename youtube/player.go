package youtube

import "regexp"

// player.go records where YouTube's player JavaScript lives. It never fetches it
// and never runs it.
//
// Until v0.4 this package carried a 463-line signature decipherer and a
// JavaScript interpreter (github.com/dop251/goja) to run YouTube's n-parameter
// function. Neither is needed: a watch page's 26 adaptive formats carry neither
// `url` nor `signatureCipher`, so there is nothing on that page to decipher, and
// the ANDROID and ANDROID_VR players answer with a plain signed `url` on every
// format. The URL below is kept on the record for provenance, so a bug report
// names the player build that served the response.
var playerJSRe = regexp.MustCompile(`/s/player/([0-9a-f]{8})/[^"'\\]+/base\.js`)

// extractPlayerJSURL returns the absolute base.js URL referenced by a watch
// page, or "" when the page does not name one.
func extractPlayerJSURL(html string) string {
	m := playerJSRe.FindString(html)
	if m == "" {
		return ""
	}
	return "https://www.youtube.com" + m
}

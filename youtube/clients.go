package youtube

import "strconv"

// clients.go is the one place a client identity is written down.
//
// A client identity is three things that have to agree: the User-Agent header,
// the X-Youtube-Client-Name and -Version headers, and the context.client object
// in the request body. When they disagree YouTube does not error, it quietly
// answers a different question. Spec 3005 doc 01 section 3.1 measured the case
// that matters: ANDROID with a mismatched header returns zero formats carrying a
// url, and ANDROID with androidSdkVersion missing returns UNPLAYABLE.
//
// So a ClientSpec fills the headers and the body from the same struct. That is a
// cheaper guarantee than a comment asking the next person to keep two lists in
// step.
type ClientSpec struct {
	// Name is the InnerTube clientName, e.g. "ANDROID".
	Name string
	// Num is the X-Youtube-Client-Name value, e.g. 3 for ANDROID.
	Num int
	// Version is both the clientVersion and X-Youtube-Client-Version value.
	Version string
	// UserAgent must match Name. The pair is what YouTube checks.
	UserAgent string
	// Host is the InnerTube host this client talks to. music.youtube.com for
	// WEB_REMIX, www.youtube.com for everything else.
	Host string
	// Extra carries the per-client fields that go into context.client beside the
	// name, version, hl and gl. ANDROID's androidSdkVersion lives here.
	Extra map[string]any
}

// Client name numbers, as YouTube assigns them. These go in the
// X-Youtube-Client-Name header and are not arbitrary.
const (
	clientNumWeb       = 1
	clientNumMWeb      = 2
	clientNumAndroid   = 3
	clientNumIOS       = 5
	clientNumWebRemix  = 67
	clientNumAndroidVR = 28
)

// Client versions. These do drift, and a stale one is a slow failure rather than
// a loud one, so they sit together where they can be bumped in one edit.
const (
	webClientVersion       = "2.20260114.08.00"
	mwebClientVersion      = "2.20260114.08.00"
	androidClientVersion   = "20.10.38"
	iosClientVersion       = "20.10.4"
	webRemixClientVersion  = "1.20260114.03.00"
	androidVRClientVersion = "1.65.10"
)

const (
	wwwHost   = "www.youtube.com"
	musicHost = "music.youtube.com"
)

// The two browser user agents. A WEB request that rotated its UA per request
// would be claiming to be three browsers in one session, which is stranger than
// claiming to be one, so these are fixed per client rather than sampled.
const (
	uaDesktopChrome = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36"
	uaMobileSafari  = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_3_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3 Mobile/15E148 Safari/604.1"
)

// ClientWEB reads pages and lists. It is the only client whose answers match
// what a person sees in a browser, and the only one that returns no stream URLs
// at all.
func ClientWEB() ClientSpec {
	return ClientSpec{
		Name:      "WEB",
		Num:       clientNumWeb,
		Version:   webClientVersion,
		UserAgent: uaDesktopChrome,
		Host:      wwwHost,
	}
}

// ClientMWEB is the mobile web client. It is kept because its comment section
// still arrives as commentRenderer rather than an entity payload.
func ClientMWEB() ClientSpec {
	return ClientSpec{
		Name:      "MWEB",
		Num:       clientNumMWeb,
		Version:   mwebClientVersion,
		UserAgent: uaMobileSafari,
		Host:      wwwHost,
	}
}

// ClientANDROID is the client to ask for streams and for caption tracks that
// return bytes. Its player response carries a plain signed url on every format,
// and its caption baseUrl values return srv3 XML where the WEB ones return 200
// and zero bytes.
//
// androidSdkVersion is not decoration. Without it the response is UNPLAYABLE.
func ClientANDROID() ClientSpec {
	return ClientSpec{
		Name:      "ANDROID",
		Num:       clientNumAndroid,
		Version:   androidClientVersion,
		UserAgent: "com.google.android.youtube/" + androidClientVersion + " (Linux; U; Android 14) gzip",
		Host:      wwwHost,
		Extra: map[string]any{
			"androidSdkVersion": 34,
			"osName":            "Android",
			"osVersion":         "14",
		},
	}
}

// ClientIOS is the second opinion on streams. It returned 27 formats with a url
// where ANDROID returned 29, so it is a fallback rather than a lead.
func ClientIOS() ClientSpec {
	return ClientSpec{
		Name:      "IOS",
		Num:       clientNumIOS,
		Version:   iosClientVersion,
		UserAgent: "com.google.ios.youtube/" + iosClientVersion + " (iPhone16,2; U; CPU iOS 18_3_2 like Mac OS X)",
		Host:      wwwHost,
		Extra: map[string]any{
			"deviceMake":  "Apple",
			"deviceModel": "iPhone16,2",
			"osName":      "iPhone",
			"osVersion":   "18.3.2.22D82",
		},
	}
}

// ClientWEBREMIX is music.youtube.com. It answers about artists, albums and
// tracks, and it says plays where www says views, which is why a music record
// and a video record stay two records.
func ClientWEBREMIX() ClientSpec {
	return ClientSpec{
		Name:      "WEB_REMIX",
		Num:       clientNumWebRemix,
		Version:   webRemixClientVersion,
		UserAgent: uaDesktopChrome,
		Host:      musicHost,
	}
}

// ClientANDROIDVR is the Oculus YouTube VR app. It is kept alongside ANDROID
// because it needs no proof-of-origin token and has been the more reliable of
// the two for the download path.
func ClientANDROIDVR() ClientSpec {
	return ClientSpec{
		Name:    "ANDROID_VR",
		Num:     clientNumAndroidVR,
		Version: androidVRClientVersion,
		UserAgent: "com.google.android.apps.youtube.vr.oculus/" + androidVRClientVersion +
			" (Linux; U; Android 12L; eureka-user Build/SQ3A.220605.009.A1) gzip",
		Host: wwwHost,
		Extra: map[string]any{
			"androidSdkVersion": 32,
			"deviceMake":        "Oculus",
			"deviceModel":       "Quest 3",
			"osName":            "Android",
			"osVersion":         "12L",
		},
	}
}

// Clients returns every client this tool claims to be, in the order `ytb
// clients` prints them. The invariant test walks this list.
func Clients() []ClientSpec {
	return []ClientSpec{
		ClientWEB(),
		ClientMWEB(),
		ClientANDROID(),
		ClientIOS(),
		ClientANDROIDVR(),
		ClientWEBREMIX(),
	}
}

// Context builds the context.client object for this spec. hl and gl go in here
// as well as on the URL and in Accept-Language, because doc 01 section 1.3
// measured that any one of the three is enough and none of them means the page
// arrives in the language of the address.
func (s ClientSpec) Context(hl, gl, visitorData string) map[string]any {
	client := map[string]any{
		"clientName":    s.Name,
		"clientVersion": s.Version,
		"hl":            hl,
		"gl":            gl,
	}
	for k, v := range s.Extra {
		client[k] = v
	}
	// The UA goes in the body too. yt-dlp found that some clients check the two
	// against each other rather than only the header.
	client["userAgent"] = s.UserAgent
	if visitorData != "" {
		client["visitorData"] = visitorData
	}
	return map[string]any{"client": client}
}

// Headers builds the request headers for this spec, from the same fields that
// filled the context. A caller cannot set one without the other.
func (s ClientSpec) Headers(visitorData string) map[string]string {
	h := map[string]string{
		"User-Agent":               s.UserAgent,
		"X-Youtube-Client-Name":    strconv.Itoa(s.Num),
		"X-Youtube-Client-Version": s.Version,
		"Origin":                   "https://" + s.Host,
		"Referer":                  "https://" + s.Host + "/",
		"Content-Type":             "application/json",
	}
	if visitorData != "" {
		h["X-Goog-Visitor-Id"] = visitorData
	}
	return h
}

// Endpoint returns the full InnerTube URL for a verb under this client's host.
func (s ClientSpec) Endpoint(verb string) string {
	return "https://" + s.Host + "/youtubei/v1/" + verb
}

// ReturnsStreamURLs reports whether this client's player response carries
// formats with a plain url. Only the mobile clients do, and `ytb clients` prints
// this column so the answer is documented rather than folklore.
func (s ClientSpec) ReturnsStreamURLs() bool {
	switch s.Name {
	case "ANDROID", "IOS", "ANDROID_VR":
		return true
	}
	return false
}

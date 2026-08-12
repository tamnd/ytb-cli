package youtube

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// session_test.go covers the two things tier 1 has to get right: reading a
// browser's cookies out of either format people actually have, and never
// letting one of them out again.

// testSAPISID is the value every assertion in this file greps for. It is not a
// real cookie and it does not have to be: what is being tested is where a string
// ends up, and a made-up string ends up in the same places a real one would.
const testSAPISID = "SAPISID-VALUE-THAT-MUST-NEVER-APPEAR-ANYWHERE"

const netscapeExport = `# Netscape HTTP Cookie File
# This is a generated file. Do not edit.

.youtube.com	TRUE	/	TRUE	1789000000	SID	sid-value
#HttpOnly_.youtube.com	TRUE	/	TRUE	1789000000	HSID	hsid-value
.youtube.com	TRUE	/	TRUE	1789000000	SAPISID	` + testSAPISID + `
.youtube.com	TRUE	/	TRUE	1789000000	__Secure-3PAPISID	third-party-value
.youtube.com	TRUE	/	TRUE	1789000000	PREF	hl=vi&gl=VN
.youtube.com	TRUE	/	TRUE	1789000000	VISITOR_INFO1_LIVE	not-a-session-cookie
.example.com	TRUE	/	TRUE	1789000000	SID	somebody-elses-sid
`

func testSession() Session {
	s := ParseCookies(netscapeExport)
	s.ImportedAt = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	s.Source = "a test"
	return s
}

func TestParseCookiesKeepsTheSessionAndDropsEverythingElse(t *testing.T) {
	s := ParseCookies(netscapeExport)

	if got := s.Cookies["SAPISID"]; got != testSAPISID {
		t.Errorf("SAPISID is %q, want the exported value", got)
	}
	if got := s.Cookies["HSID"]; got != "hsid-value" {
		t.Errorf("the #HttpOnly_ prefix hid a cookie: HSID is %q", got)
	}
	if _, ok := s.Cookies["VISITOR_INFO1_LIVE"]; ok {
		t.Error("a cookie outside the allowlist was kept, so the export decides what ytb sends")
	}
	// PREF is excluded on purpose: locale.go sets it per request from --hl, and a
	// stored copy would send hl=vi on every read and give back Vietnamese pages.
	if _, ok := s.Cookies["PREF"]; ok {
		t.Error("PREF was kept, which would override --hl on every request")
	}
	if s.Cookies["SID"] != "sid-value" {
		t.Errorf("a cookie from another domain won: SID is %q", s.Cookies["SID"])
	}
	if s.Empty() {
		t.Errorf("that export is a working session and Empty says otherwise: missing %v", s.missing())
	}
}

func TestParseCookiesReadsAPastedHeader(t *testing.T) {
	s := ParseCookies("SID=sid-value; SAPISID=" + testSAPISID + "; YSC=drop-me")
	if s.Empty() {
		t.Fatalf("a pasted header did not parse: missing %v", s.missing())
	}
	if _, ok := s.Cookies["YSC"]; ok {
		t.Error("the allowlist did not apply to the header form")
	}
}

// TestAnIncompleteExportIsRefused is the case that would otherwise fail against
// the live site with a 401 and no explanation. Half a session is not a session,
// and saying which half is missing is the whole diagnosis.
func TestAnIncompleteExportIsRefused(t *testing.T) {
	s := ParseCookies("SID=sid-value; LOGIN_INFO=something")
	if !s.Empty() {
		t.Fatal("a session with no SAPISID reports itself usable")
	}
	if got := s.missing(); len(got) != 1 || got[0] != "SAPISID" {
		t.Errorf("missing says %v, want just SAPISID", got)
	}
	if err := SaveSession(t.TempDir(), s); err == nil {
		t.Error("an unusable session was saved, so the failure moves to the next command")
	}
}

func TestAuthorizationIsRecomputedPerRequest(t *testing.T) {
	s := testSession()
	origin := "https://www.youtube.com"

	first := s.Authorization(origin, time.Unix(1700000000, 0))
	if !strings.HasPrefix(first, "SAPISIDHASH 1700000000_") {
		t.Fatalf("the header is %q, want SAPISIDHASH <ts>_<sha1>", first)
	}
	if strings.Contains(first, testSAPISID) {
		t.Errorf("the header carries the cookie itself: %q", first)
	}
	if later := s.Authorization(origin, time.Unix(1700000060, 0)); later == first {
		t.Error("two requests a minute apart produced the same header, so the timestamp is not in it")
	}
	// The origin is in the hash, so music.youtube.com cannot reuse www's.
	if other := s.Authorization("https://music.youtube.com", time.Unix(1700000000, 0)); other == first {
		t.Error("two origins hash the same, so the origin is not in the digest")
	}
	if (Session{}).Authorization(origin, time.Unix(1700000000, 0)) != "" {
		t.Error("a session with no cookies produced an Authorization header")
	}
}

func TestSessionFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSession(dir, testSession()); err != nil {
		t.Fatalf("save: %v", err)
	}
	fi, err := os.Stat(SessionPath(dir))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := fi.Mode().Perm(); mode != fs.FileMode(0o600) {
		t.Errorf("the session file is %04o, which somebody else on this machine can read; want 0600", mode)
	}
}

func TestLoadSessionRoundTripsAndClearForgets(t *testing.T) {
	dir := t.TempDir()

	// No file at all is signed out, not an error. It is the normal case.
	if s, err := LoadSession(dir); err != nil || !s.Empty() {
		t.Fatalf("an empty data directory gave (%v, %v), want an empty session and no error", s.Cookies, err)
	}
	if err := SaveSession(dir, testSession()); err != nil {
		t.Fatalf("save: %v", err)
	}
	back, err := LoadSession(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if back.Cookies["SAPISID"] != testSAPISID || back.Source != "a test" {
		t.Errorf("the session did not survive the round trip: %+v", back.names())
	}
	if err := ClearSession(dir); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if s, _ := LoadSession(dir); !s.Empty() {
		t.Error("the session is still there after clear")
	}
	// Clearing twice is the state the caller asked for, both times.
	if err := ClearSession(dir); err != nil {
		t.Errorf("clearing a session that is already gone failed: %v", err)
	}
}

func TestStatusNamesTheCookiesAndHoldsNoValues(t *testing.T) {
	st := testSession().Status("/tmp/session.json")
	if !st.Present || st.Tier != 1 {
		t.Fatalf("status says present=%v tier=%d for a working session", st.Present, st.Tier)
	}
	if len(st.Unlocks) == 0 {
		t.Error("status says nothing about what the session unlocks, which is what it is for")
	}
	for _, name := range st.Cookies {
		if strings.Contains(name, testSAPISID) {
			t.Fatalf("a cookie value reached the status: %q", name)
		}
	}
	if !contains(st.Cookies, "SAPISID") {
		t.Errorf("status does not name SAPISID: %v", st.Cookies)
	}
}

// TestTheSessionIsNeverSentToTheCDN is the one host that must not see it. A
// googlevideo URL is signed already and the CDN has no idea who you are; sending
// an account cookie there would attach a name to every byte range for no gain.
func TestTheSessionIsNeverSentToTheCDN(t *testing.T) {
	c := NewClient(DefaultConfig())
	c.SetSession(testSession())

	media, _ := http.NewRequest(http.MethodGet, "https://rr3---sn-abc.googlevideo.com/videoplayback?expire=1", nil)
	c.applySession(media)
	if got := media.Header.Get("Cookie"); got != "" {
		t.Errorf("a media request carries a cookie: %q", got)
	}
	if media.Header.Get("Authorization") != "" {
		t.Error("a media request carries an Authorization header")
	}

	watch, _ := http.NewRequest(http.MethodGet, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", nil)
	c.setLanguageHeaders(watch)
	c.applySession(watch)
	cookie := watch.Header.Get("Cookie")
	if !strings.Contains(cookie, "SAPISID=") {
		t.Errorf("a watch request carries no session: %q", cookie)
	}
	// locale.go put PREF on this request before the session went on. Losing it
	// would bring back the Vietnamese page, and only for signed-in reads.
	if !strings.Contains(cookie, "PREF=") {
		t.Errorf("the session overwrote the language cookies: %q", cookie)
	}
	if !strings.HasPrefix(watch.Header.Get("Authorization"), "SAPISIDHASH ") {
		t.Errorf("a watch request carries no Authorization: %q", watch.Header.Get("Authorization"))
	}
}

// TestTheCacheKeyKnowsAboutTheSession is the bug this would otherwise have: an
// age-restricted watch page fetched signed in, then served from the cache to a
// signed-out read, which would report a refusal as a video or the other way
// round with nothing on the record to say so.
func TestTheCacheKeyKnowsAboutTheSession(t *testing.T) {
	anon := NewClient(DefaultConfig())
	signedIn := NewClient(DefaultConfig())
	signedIn.SetSession(testSession())

	other := ParseCookies("SID=sid-value; SAPISID=a-different-account")
	second := NewClient(DefaultConfig())
	second.SetSession(other)

	a, b, cc := anon.cacheClient("WEB/html"), signedIn.cacheClient("WEB/html"), second.cacheClient("WEB/html")
	if a == b {
		t.Error("a signed-in read and a signed-out read share a cache key")
	}
	if b == cc {
		t.Error("two accounts share a cache key, so switching accounts serves one the other's pages")
	}
	if strings.Contains(b, testSAPISID) {
		t.Errorf("the cache key carries the cookie, and a cache key becomes a filename: %q", b)
	}
	if signedIn.Tier() != 1 || anon.Tier() != 0 {
		t.Errorf("tier is %d signed in and %d signed out", signedIn.Tier(), anon.Tier())
	}
}

// TestTheReadsLogRedactsWhatWasSent covers `ytb archive`, which writes the
// request headers down beside the payload so a record can be checked against
// what was asked for. Those headers include the session.
func TestTheReadsLogRedactsWhatWasSent(t *testing.T) {
	c := NewClient(DefaultConfig())
	c.SetSession(testSession())
	req, _ := http.NewRequest(http.MethodGet, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", nil)
	c.setLanguageHeaders(req)
	c.applySession(req)

	for k, v := range sentHeaders(req.Header) {
		if strings.Contains(v, testSAPISID) {
			t.Errorf("the archived %s header carries the session: %q", k, v)
		}
	}
	if got := sentHeaders(req.Header)["Cookie"]; got == "" {
		t.Error("the Cookie header vanished from the log instead of being replaced, so nothing says one was sent")
	}
}

func TestStampMarksTheTierAndTheSurface(t *testing.T) {
	c := NewClient(DefaultConfig())
	v := &Video{VideoID: "dQw4w9WgXcQ", Envelope: newEnvelope("video", SurfaceWatchHTML)}

	c.stamp(v)
	if v.Tier != 0 || v.Surfaces.Has(SurfaceSession) {
		t.Errorf("a signed-out read stamped tier %d with surfaces %v", v.Tier, v.Surfaces)
	}

	c.SetSession(testSession())
	c.stamp(v)
	if v.Tier != 1 {
		t.Errorf("a signed-in read left the record at tier %d", v.Tier)
	}
	if !v.Surfaces.Has(SurfaceSession) {
		t.Errorf("the record does not name s11: %v", v.Surfaces)
	}
	if !v.Surfaces.Has(SurfaceWatchHTML) {
		t.Errorf("the stamp dropped the surface that answered: %v", v.Surfaces)
	}

	// A page of records and a mixed stream go through their own helpers, and
	// both of them have to end up here.
	rows := []Video{{VideoID: "a", Envelope: newEnvelope("video", SurfaceFeed)}}
	stampAll(c, rows)
	if rows[0].Tier != 1 {
		t.Errorf("stampAll left a row at tier %d", rows[0].Tier)
	}
	mixed, ok := c.stampAny(Channel{ChannelID: "UC", Envelope: newEnvelope("channel", SurfaceBrowseHTML)}).(Channel)
	if !ok || mixed.Tier != 1 {
		t.Errorf("stampAny returned %T at tier %d", mixed, mixed.Tier)
	}
}

// TestNothingOnDiskHoldsTheCookie is the fourth box of milestone 14: after a
// signed-in run, the only file under the data directory with a cookie value in
// it is the session file.
//
// It writes what a signed-in run writes, which is a cache entry under a
// session-marked key, a node and a read in the store, and the archived request
// headers, and then greps every byte of the directory. A blanket walk is the
// point: a test that checks the three files it knows about passes on the day
// somebody adds a fourth.
func TestNothingOnDiskHoldsTheCookie(t *testing.T) {
	dir := t.TempDir()
	if err := SaveSession(dir, testSession()); err != nil {
		t.Fatalf("save the session: %v", err)
	}

	c := NewClient(DefaultConfig())
	c.SetSession(testSession())

	// The cache: the key becomes the filename and the body is what came back, so
	// a session in either would be a session on disk twice.
	cache := NewCache(filepath.Join(dir, "cache", "http"), time.Hour)
	key := CacheKey{
		Method: http.MethodGet,
		URL:    "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Client: c.cacheClient("WEB/html"),
	}
	cache.Put(key, 200, []byte(`{"page":"an age-restricted watch page, which is why the session was sent"}`))

	// The store: a record and its read, which is what ytb crawl leaves behind.
	store, err := OpenStore(filepath.Join(dir, "ytb.db"))
	if err != nil {
		t.Fatalf("open the store: %v", err)
	}
	video := &Video{VideoID: "dQw4w9WgXcQ", Title: "a signed-in read", Envelope: newEnvelope("video", SurfaceWatchHTML)}
	c.stamp(video)
	if _, err := store.PutRecord(video); err != nil {
		t.Fatalf("put the record: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, key.URL, nil)
	c.setLanguageHeaders(req)
	c.applySession(req)
	read := Read{
		Method:  req.Method,
		URL:     key.URL,
		Surface: SurfaceWatchHTML,
		Client:  "WEB",
		Status:  200,
		Headers: sentHeaders(req.Header),
		At:      time.Now(),
	}
	if err := store.PutRead(read); err != nil {
		t.Fatalf("put the read: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}

	// The archive: the headers written down beside the payload.
	archive := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archive, 0o755); err != nil {
		t.Fatal(err)
	}
	blob, err := json.MarshalIndent(read, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, "reads.json"), blob, 0o644); err != nil {
		t.Fatal(err)
	}

	sessionFile := SessionPath(dir)
	var leaked []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), testSAPISID) && !strings.Contains(path, testSAPISID) {
			return nil
		}
		if path == sessionFile {
			return nil
		}
		leaked = append(leaked, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(leaked) > 0 {
		t.Errorf("a cookie value is on disk outside the session file:\n%s", strings.Join(leaked, "\n"))
	}

	// And the session file does hold it, so a walk that found nothing was a walk
	// over something rather than a walk over an empty directory.
	stored, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stored), testSAPISID) {
		t.Fatal("the session file does not hold the cookie either, so this test proves nothing")
	}
}

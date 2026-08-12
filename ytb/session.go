package ytb

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// session.go holds the cookies tier 1 needs and turns them into two headers.
//
// Tier 0 is the product and this file is the exception. Everything ytb reads
// without a credential it keeps reading without one; a session adds the five
// things doc 01 section 12 names and nothing else: comments on a network where
// Restricted Mode is on, the community tab of a channel that hides it, an
// age-restricted video, members-only content, and your own subscriptions,
// playlists and history.
//
// ytb never asks for a password, never drives a login form and never touches a
// consent screen. You export the cookies your browser already has and hand the
// file over. They live 0600 in the data directory, they go into a request
// header and nowhere else, and there is a test that greps every output format,
// the cache, the store and an archive directory for them.

// sessionCookies is what ytb keeps out of the sixty a browser holds for
// youtube.com. It is an allowlist rather than a blocklist: a cookies.txt
// exported from a logged-in browser carries advertising and experiment cookies
// that have nothing to do with the account, and keeping only the named ones
// means a file that grows a new cookie next year does not silently start
// sending it.
//
// PREF, CONSENT and SOCS are deliberately not here. They carry the interface
// language and the consent state, locale.go sets all three per request, and a
// stored copy would quietly override the --hl the user just passed.
var sessionCookies = []string{
	"APISID",
	"HSID",
	"LOGIN_INFO",
	"SAPISID",
	"SID",
	"SIDCC",
	"SSID",
	"__Secure-1PAPISID",
	"__Secure-1PSID",
	"__Secure-1PSIDCC",
	"__Secure-1PSIDTS",
	"__Secure-3PAPISID",
	"__Secure-3PSID",
	"__Secure-3PSIDCC",
	"__Secure-3PSIDTS",
}

// Session is an imported browser session.
//
// It marshals with its values in it, because that is what the 0600 file on disk
// has to hold. Nothing else renders a Session: `ytb auth status` renders a
// SessionStatus, which has names in it and no values.
type Session struct {
	Cookies    map[string]string `json:"cookies"`
	ImportedAt time.Time         `json:"imported_at,omitzero"`
	// Source is where the cookies were read from, for the person who comes back
	// in three months and wants to know which browser profile this was.
	Source string `json:"source,omitempty"`
}

// SessionStatus is what `ytb auth status` prints. It is a separate type
// because the obvious way to render a Session is to marshal it, and marshalling
// a Session prints the session.
type SessionStatus struct {
	Present bool `json:"present" table:"present"`
	// Tier is what a read would be at with this session loaded, so status and a
	// record's envelope say the same number.
	Tier int `json:"tier" table:"tier"`
	// Cookies names what is stored. Names only.
	Cookies []string `json:"cookies,omitempty" table:"cookies"`
	// Missing names the cookies that are not stored and would be needed. It is
	// empty for a session that works and is the whole diagnosis for one that
	// does not.
	Missing    []string  `json:"missing,omitempty" table:"missing"`
	Unlocks    []string  `json:"unlocks,omitempty" table:"-"`
	Source     string    `json:"source,omitempty" table:"source"`
	ImportedAt time.Time `json:"imported_at,omitzero" table:"imported"`
	Path       string    `json:"path" table:"-"`
}

// tier1Unlocks is what doc 01 section 12 promises a session buys, in the words
// of the thing the user was trying to read when they went looking for this.
var tier1Unlocks = []string{
	"comments on a network with Restricted Mode on",
	"the community tab of a channel that gates it",
	"age-restricted videos",
	"members-only videos and posts",
	"your own subscriptions, playlists and history",
}

// Empty reports whether there is no usable session here.
//
// Usable is a specific thing: the SAPISID pair signs the request and the SID
// pair identifies the account, and one without the other authenticates nothing.
// A file with ten cookies in it and neither pair complete is empty, and status
// says which half is missing rather than reporting a session that will 401.
func (s Session) Empty() bool { return len(s.missing()) > 0 }

// missing names the cookies a session needs and does not have. Either member of
// each pair will do: a browser signed in normally has both, and an export from
// a container or a partitioned profile sometimes has only the __Secure-3P one.
func (s Session) missing() []string {
	var out []string
	if s.value("SAPISID") == "" && s.value("__Secure-3PAPISID") == "" {
		out = append(out, "SAPISID")
	}
	if s.value("SID") == "" && s.value("__Secure-3PSID") == "" {
		out = append(out, "SID")
	}
	return out
}

func (s Session) value(name string) string { return strings.TrimSpace(s.Cookies[name]) }

// names lists what is stored, sorted, so two runs print the same row.
func (s Session) names() []string {
	out := make([]string, 0, len(s.Cookies))
	for k, v := range s.Cookies {
		if strings.TrimSpace(v) != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Header is the Cookie header value a request carries.
func (s Session) Header() string {
	names := s.names()
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+"="+s.Cookies[n])
	}
	return strings.Join(parts, "; ")
}

// sapisid is the cookie the Authorization header is computed from. The plain
// one is preferred and the third-party one is the fallback, which is the order
// the site itself tries them in.
func (s Session) sapisid() string {
	if v := s.value("SAPISID"); v != "" {
		return v
	}
	return s.value("__Secure-3PAPISID")
}

// Authorization is the header YouTube authenticates an InnerTube call with.
//
// It is not a token. It is a SHA-1 of the timestamp, the SAPISID cookie and the
// origin, joined by spaces, and it is recomputed for every request because the
// timestamp is in it and the server checks how old it is. Nothing derived from
// it is stored: there is no token file, because there is no token.
func (s Session) Authorization(origin string, now time.Time) string {
	sapisid := s.sapisid()
	if sapisid == "" {
		return ""
	}
	ts := now.Unix()
	sum := sha1.Sum(fmt.Appendf(nil, "%d %s %s", ts, sapisid, origin))
	return fmt.Sprintf("SAPISIDHASH %d_%s", ts, hex.EncodeToString(sum[:]))
}

// marker is a short stable fingerprint of the session, for the cache key.
//
// It is a hash and not the cookie, because a cache key becomes a filename and a
// filename is not a place to put a session. Two accounts get two markers, so
// switching accounts does not serve one's watch page to the other, and signing
// out goes back to the unmarked keys the tier 0 reads already wrote.
func (s Session) marker() string {
	if s.Empty() {
		return ""
	}
	sum := sha256.Sum256([]byte(s.sapisid()))
	return hex.EncodeToString(sum[:4])
}

// SessionPath is where the cookies live for a data directory.
func SessionPath(dataDir string) string {
	return filepath.Join(dataDir, "session.json")
}

// LoadSession reads the stored session. No file is not an error: running signed
// out is the normal case, and tier 0 is nearly all of what ytb does.
func LoadSession(dataDir string) (Session, error) {
	if dataDir == "" {
		return Session{}, nil
	}
	b, err := os.ReadFile(SessionPath(dataDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Session{}, nil
		}
		return Session{}, err
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return Session{}, fmt.Errorf("the session file at %s is not readable: %w", SessionPath(dataDir), err)
	}
	return s, nil
}

// SaveSession writes the session 0600, and the directory 0700, because a
// world-readable directory around a 0600 file still tells everyone the file is
// there.
func SaveSession(dataDir string, s Session) error {
	if dataDir == "" {
		return errors.New("no data directory to write the session into")
	}
	if s.Empty() {
		return fmt.Errorf("that export has no session in it: %s missing", strings.Join(s.missing(), " and "))
	}
	path := SessionPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// ClearSession forgets the session. Forgetting one that is not there is not an
// error, because the state the caller asked for is the state they end up in.
func ClearSession(dataDir string) error {
	err := os.Remove(SessionPath(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Status describes the stored session without printing it.
func (s Session) Status(path string) SessionStatus {
	st := SessionStatus{
		Cookies:    s.names(),
		Missing:    s.missing(),
		Source:     s.Source,
		ImportedAt: s.ImportedAt,
		Path:       path,
	}
	if !s.Empty() {
		st.Present, st.Tier, st.Unlocks = true, 1, tier1Unlocks
	}
	return st
}

// ReadSession reads cookies from a file, from stdin, or from a header string.
//
// All three forms show up in practice. A browser extension exports a Netscape
// cookies.txt, the devtools network panel copies a Cookie header, and a script
// pipes either one in. Guessing between them is safe here because the two
// formats cannot be confused: a Netscape line has tabs in it and a header line
// has semicolons.
func ReadSession(arg string) (Session, error) {
	source := arg
	var raw []byte
	switch {
	case arg == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return Session{}, err
		}
		raw, source = b, "stdin"
	case looksLikeCookieHeader(arg):
		raw, source = []byte(arg), "a pasted header"
	default:
		b, err := os.ReadFile(arg)
		if err != nil {
			return Session{}, fmt.Errorf("read cookies: %w", err)
		}
		abs, aerr := filepath.Abs(arg)
		if aerr == nil {
			source = abs
		}
		raw = b
	}
	s := ParseCookies(string(raw))
	s.Source = source
	s.ImportedAt = time.Now().UTC()
	return s, nil
}

// looksLikeCookieHeader reports whether the argument is the cookies themselves
// rather than a path to them. A name=value pair with no path separator in front
// of it is a header; anything else is a filename and is opened as one, so a
// typo in a path fails as a missing file rather than as an empty session.
func looksLikeCookieHeader(arg string) bool {
	if !strings.Contains(arg, "=") {
		return false
	}
	head, _, _ := strings.Cut(arg, "=")
	return !strings.ContainsAny(head, "/\\") && strings.TrimSpace(head) != ""
}

// ParseCookies reads either format and keeps the cookies in sessionCookies.
func ParseCookies(text string) Session {
	s := Session{Cookies: map[string]string{}}
	keep := map[string]bool{}
	for _, n := range sessionCookies {
		keep[n] = true
	}
	for line := range strings.Lines(text) {
		eachCookie(strings.TrimRight(line, "\r\n"), func(name, value string) {
			if keep[name] && value != "" {
				s.Cookies[name] = value
			}
		})
	}
	return s
}

// eachCookie calls fn for every cookie on a line of either format.
func eachCookie(line string, fn func(name, value string)) {
	// A Netscape line is seven tab-separated fields: domain, a host-only flag,
	// path, secure, expiry, name, value, and it holds exactly one cookie. The
	// `#HttpOnly_` prefix is a curl extension on the domain field, and a comment
	// is anything else starting with a hash.
	if strings.Contains(line, "\t") {
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			return
		}
		domain := strings.TrimPrefix(strings.TrimSpace(fields[0]), "#HttpOnly_")
		if strings.HasPrefix(domain, "#") || !cookieDomainIsGoogle(domain) {
			return
		}
		fn(strings.TrimSpace(fields[5]), strings.TrimSpace(fields[6]))
		return
	}
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return
	}
	// Otherwise a Cookie header: name=value pairs separated by semicolons, all
	// on one line, which is how devtools copies it. There is no domain on a
	// header, so there is nothing to check it against; the allowlist is the only
	// filter this form gets, and it is the one that matters.
	for _, part := range strings.Split(line, ";") {
		n, v, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		fn(strings.TrimSpace(n), strings.TrimSpace(v))
	}
}

// cookieDomainIsGoogle keeps a cookies.txt exported for the whole browser from
// contributing whatever the user's bank set. Only youtube.com and google.com
// cookies are read, and only the named ones out of those.
func cookieDomainIsGoogle(domain string) bool {
	d := strings.ToLower(strings.TrimPrefix(domain, "."))
	return d == "youtube.com" || strings.HasSuffix(d, ".youtube.com") ||
		d == "google.com" || strings.HasSuffix(d, ".google.com")
}

// applySession puts the session on a request.
//
// Two headers do the work: Cookie carries the account and Authorization carries
// the per-request SAPISIDHASH. X-Origin has to agree with the origin the hash
// was computed over or the server rejects both.
//
// It is gated on isYouTubeHost, so nothing here reaches googlevideo. A media URL
// is signed already, it is served by a CDN that has no idea who you are, and
// sending an account cookie to it would attach a name to every byte range for
// no gain at all.
func (c *Client) applySession(req *http.Request) {
	if c == nil || c.session.Empty() || req == nil || req.URL == nil {
		return
	}
	if !isYouTubeHost(req.URL.Host) {
		return
	}
	origin := req.URL.Scheme + "://" + req.URL.Host
	// The cookies are appended rather than set, because setLanguageHeaders has
	// already put PREF, SOCS and CONSENT on this request and a Set here would
	// drop all three. Losing PREF is the Vietnamese page locale.go exists to
	// prevent, and it would come back only for signed-in reads, which is the
	// hardest kind of bug to see.
	for _, name := range c.session.names() {
		req.AddCookie(&http.Cookie{Name: name, Value: c.session.Cookies[name]})
	}
	req.Header.Set("Authorization", c.session.Authorization(origin, time.Now()))
	req.Header.Set("X-Origin", origin)
	req.Header.Set("X-Goog-AuthUser", "0")
}

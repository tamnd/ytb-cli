package youtube

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// policy_test.go asserts the promises the tool makes about itself.
//
// These are not unit tests. Each one is a rule that a future change could break
// quietly, where the damage is done before anyone notices: a hardcoded API key
// that works until it rotates, a media request without a Range header that runs
// at 32 KiB/s, a handcrafted continuation token that returns a plausible wrong
// answer. A test is the only place a rule like that stays enforced.

// repoFiles returns every .go file in the module, tests included.
func repoFiles(t *testing.T) []string {
	t.Helper()
	root := ".."
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "docs", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	if len(out) < 10 {
		t.Fatalf("only found %d go files, the walk is wrong", len(out))
	}
	return out
}

// stringLiterals returns every string literal in a file, with its position.
func stringLiterals(t *testing.T, path string) map[string]token.Position {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]token.Position{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if _, seen := out[s]; !seen {
			out[s] = fset.Position(lit.Pos())
		}
		return true
	})
	return out
}

// apiKeyRe is the shape of a Google API key. The two YouTube keys are recorded in
// spec 3005 doc 01 section 10 and deliberately not here: this test would fail on
// its own fixture.
var apiKeyRe = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)

// TestNoAPIKeyLiteral is the load-bearing one. The key is harvested from ytcfg
// on the first page of a run, so there is no reason for one to appear in the
// source, and a hardcoded key is a tool that stops working on a day nobody
// changed anything.
func TestNoAPIKeyLiteral(t *testing.T) {
	for _, path := range repoFiles(t) {
		for lit, pos := range stringLiterals(t, path) {
			if apiKeyRe.MatchString(lit) {
				t.Errorf("%s: API key literal in source; harvest it from ytcfg instead", pos)
			}
		}
	}
}

// TestNoJavaScriptRuntime asserts the JS interpreter stays gone. A watch page's
// adaptive formats carry no signatureCipher, so there is nothing to run, and a
// dependency on a JS runtime is a large attack surface for a feature we do not
// have.
func TestNoJavaScriptRuntime(t *testing.T) {
	banned := []string{
		"github.com/dop251/goja",
		"github.com/robertkrimen/otto",
		"rogchap.com/v8go",
	}
	mod, err := os.ReadFile("../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	for _, dep := range banned {
		if strings.Contains(string(mod), dep) {
			t.Errorf("go.mod depends on %s; nothing in this tool runs JavaScript", dep)
		}
	}
}

// TestNoWriteEndpoint asserts no InnerTube verb that changes state is named
// anywhere. This tool reads. A like, a subscribe or a comment post needs a
// credential and an intent this tool does not have, and naming the endpoint is
// the first step to calling it by accident.
func TestNoWriteEndpoint(t *testing.T) {
	banned := []string{
		"like/like",
		"like/dislike",
		"like/removelike",
		"subscription/subscribe",
		"subscription/unsubscribe",
		"comment/create_comment",
		"comment/perform_comment_action",
		"playlist/create",
		"playlist/delete",
		"browse/edit_playlist",
	}
	for _, path := range repoFiles(t) {
		// This file holds the list, so it names all of them by construction.
		if filepath.Base(path) == "policy_test.go" {
			continue
		}
		for lit, pos := range stringLiterals(t, path) {
			low := strings.ToLower(lit)
			for _, verb := range banned {
				if strings.Contains(low, verb) {
					t.Errorf("%s: names the write endpoint %q; this tool only reads", pos, verb)
				}
			}
		}
	}
}

// TestClientSpecsAreCoherent asserts the thing that cannot be checked at
// runtime: that a client's user agent matches its name. A mismatch is how
// ANDROID stops returning stream URLs, and it fails silently.
func TestClientSpecsAreCoherent(t *testing.T) {
	seenNum := map[int]string{}
	for _, s := range Clients() {
		if s.Name == "" || s.Version == "" || s.UserAgent == "" || s.Host == "" {
			t.Errorf("client %q has an empty field", s.Name)
		}
		if prev, ok := seenNum[s.Num]; ok && prev != s.Name {
			t.Errorf("clients %s and %s share client number %d", prev, s.Name, s.Num)
		}
		seenNum[s.Num] = s.Name

		// The version in the UA has to be the version we claim, or the pair
		// YouTube checks does not agree with itself.
		if strings.HasPrefix(s.UserAgent, "com.google.") && !strings.Contains(s.UserAgent, s.Version) {
			t.Errorf("client %s claims version %s but its user agent does not carry it: %s",
				s.Name, s.Version, s.UserAgent)
		}
		switch s.Name {
		case "ANDROID", "ANDROID_VR":
			if _, ok := s.Extra["androidSdkVersion"]; !ok {
				t.Errorf("client %s has no androidSdkVersion; without it the player response is UNPLAYABLE", s.Name)
			}
		case "WEB_REMIX":
			if s.Host != musicHost {
				t.Errorf("client WEB_REMIX must talk to %s, has %s", musicHost, s.Host)
			}
		}
	}
}

// TestAndroidContextCarriesSDKVersion asserts androidSdkVersion reaches the wire
// rather than only the struct. Spec 3005 doc 01 section 3.1: without it the
// response degrades to UNPLAYABLE.
func TestAndroidContextCarriesSDKVersion(t *testing.T) {
	ctx := ClientANDROID().Context("en", "US", "")
	client, ok := ctx["client"].(map[string]any)
	if !ok {
		t.Fatal("context has no client object")
	}
	if got := client["androidSdkVersion"]; got != 34 {
		t.Errorf("androidSdkVersion = %v, want 34", got)
	}
	if got := client["clientName"]; got != "ANDROID" {
		t.Errorf("clientName = %v, want ANDROID", got)
	}
	if client["hl"] != "en" || client["gl"] != "US" {
		t.Errorf("hl/gl missing from context: %v/%v", client["hl"], client["gl"])
	}
}

// TestHeadersMatchContext asserts the one-struct rule holds: the headers and the
// body agree because they came from the same place.
func TestHeadersMatchContext(t *testing.T) {
	for _, s := range Clients() {
		h := s.Headers("")
		ctx, _ := s.Context("en", "US", "")["client"].(map[string]any)
		if h["X-Youtube-Client-Version"] != ctx["clientVersion"] {
			t.Errorf("%s: header version %q != context version %q",
				s.Name, h["X-Youtube-Client-Version"], ctx["clientVersion"])
		}
		if h["User-Agent"] != ctx["userAgent"] {
			t.Errorf("%s: header UA != context UA", s.Name)
		}
		if h["X-Youtube-Client-Name"] != strconv.Itoa(s.Num) {
			t.Errorf("%s: header client name %q != %d", s.Name, h["X-Youtube-Client-Name"], s.Num)
		}
	}
}

// TestCacheKeyIncludesClient is the one that stops a silent wrong answer. The
// same player URL returns caption tracks that yield bytes to ANDROID and caption
// tracks that yield nothing to WEB, so an entry shared between them serves an
// empty transcript with no error to explain it.
func TestCacheKeyIncludesClient(t *testing.T) {
	base := CacheKey{Method: "POST", URL: "https://www.youtube.com/youtubei/v1/player", Body: []byte(`{"videoId":"x"}`)}
	web := base
	web.Client = "WEB/2.0"
	android := base
	android.Client = "ANDROID/20.10.38"
	if web.String() == android.String() {
		t.Error("cache key does not distinguish clients; a WEB player response could be served to a caption read")
	}

	// A body change has to move the key too, or two searches share one entry.
	other := web
	other.Body = []byte(`{"videoId":"y"}`)
	if other.String() == web.String() {
		t.Error("cache key ignores the request body")
	}
}

// TestRefusalsAreNotRetried asserts a refusal is recognisable as one, so the
// retry loop and the exit code both treat it as an answer.
func TestRefusalsAreNotRetried(t *testing.T) {
	r := newRefusal("comments for kJQP7kiw5Fk", "next", "Restricted Mode has hidden comments for this video.")
	if !IsRefusal(r) {
		t.Error("a Refusal is not reported as one")
	}
	if r.Remedy == "" {
		t.Error("the Restricted Mode refusal carries no remedy, so the user is told nothing actionable")
	}
	if !strings.Contains(r.Error(), "Restricted Mode has hidden comments") {
		t.Errorf("the refusal does not quote YouTube: %s", r.Error())
	}
	// A plain error must not be mistaken for a refusal, or a 503 stops the run.
	if IsRefusal(errTest) {
		t.Error("a plain error is reported as a refusal")
	}
}

var errTest = &testErr{}

type testErr struct{}

func (*testErr) Error() string { return "connection reset" }

// TestLocaliseAddsLanguage asserts hl and gl reach the URL, which is one of the
// three places they go. Doc 01 section 1.3: with none of them the page arrives in
// the language of the requesting address, and a parser expecting English reports
// an empty channel.
func TestLocaliseAddsLanguage(t *testing.T) {
	c := NewClient(Config{HL: "en", GL: "US"})
	got := c.localise("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	for _, want := range []string{"hl=en", "gl=US", "v=dQw4w9WgXcQ"} {
		if !strings.Contains(got, want) {
			t.Errorf("localise dropped %s: %s", want, got)
		}
	}

	// An explicit hl is the caller meaning it, and must survive.
	got = c.localise("https://www.youtube.com/watch?v=x&hl=de")
	if !strings.Contains(got, "hl=de") {
		t.Errorf("localise overrode an explicit hl: %s", got)
	}

	// A signed media URL must not be touched. Adding a parameter to it
	// invalidates the signature.
	media := "https://rr3---sn-abc.googlevideo.com/videoplayback?expire=1&sig=2"
	if c.localise(media) != media {
		t.Errorf("localise modified a signed media URL: %s", c.localise(media))
	}
}

// protobufBuilders are the calls that mean "a protobuf blob was assembled here
// rather than read off a response".
var protobufBuilders = []string{
	"proto.Marshal",
	"base64.StdEncoding.EncodeToString",
	"base64.URLEncoding.EncodeToString",
	"base64.RawURLEncoding.EncodeToString",
}

// blobBuildersAllowed is every file permitted to build one, with why.
//
// The distinction this test rests on: a search filter `sp` blob and a
// continuation token are both base64 protobufs, and they are not the same kind
// of thing. The filter grid is a fixed, small, documented set of options, and
// constructing it is how `--duration long` works at all. A continuation token
// encodes server state we cannot see, and a handcrafted one returned a
// backgroundPromoRenderer saying "This video isn't available anymore" for a
// video that was fine, which is a wrong answer rather than an error.
var blobBuildersAllowed = map[string]string{
	"search_params.go": "builds the search filter sp blob, which is a fixed option set and not server state",
}

// TestNoConstructedContinuationToken asserts continuation tokens are only ever
// read from a response, by requiring any file that assembles a protobuf blob to
// be on an allowlist with a stated reason.
func TestNoConstructedContinuationToken(t *testing.T) {
	for _, path := range repoFiles(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(src)
		for _, b := range protobufBuilders {
			if !strings.Contains(text, b) {
				continue
			}
			if _, ok := blobBuildersAllowed[filepath.Base(path)]; ok {
				continue
			}
			t.Errorf("%s uses %s. A continuation token must come off a response, never be built. "+
				"If this blob is not a continuation token, add the file to blobBuildersAllowed with the reason.",
				path, b)
		}
	}
}

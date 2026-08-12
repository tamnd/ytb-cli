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

// TestEveryStreamRequestSetsRange is the source-level half of the media rule.
// download_test.go proves the requests that actually go out carry a Range
// header; this one proves there is no function that could send one without it,
// including a path no test happens to walk.
//
// Doc 01 section 8: the same URL fetched un-ranged is throttled to 32 KiB/s and
// never finishes, so this is not a performance note, it is whether the tool
// works. The failure it guards against is a well-meant "fall back to a plain GET
// when contentLength is missing", which reads like robustness and is the bug.
func TestEveryStreamRequestSetsRange(t *testing.T) {
	const path = "download.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	requestBuilders := 0
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var builds, ranges int
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				switch sel.Sel.Name {
				case "NewRequest", "NewRequestWithContext", "Get", "Head", "Post":
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "http" {
						builds++
					}
				case "Set", "Add":
					if len(call.Args) > 0 {
						if lit, ok := call.Args[0].(*ast.BasicLit); ok {
							if s, err := strconv.Unquote(lit.Value); err == nil && s == "Range" {
								ranges++
							}
						}
					}
				}
			}
			return true
		})
		if builds == 0 {
			continue
		}
		requestBuilders++
		if ranges == 0 {
			t.Errorf("%s builds an HTTP request and never sets a Range header, which is the 32 KiB/s path",
				fset.Position(fn.Pos()))
		}
	}
	// One place builds requests here, on purpose. Two would mean the rule has to
	// hold in two places, and the second one is where it stops holding.
	if requestBuilders != 1 {
		t.Errorf("%s has %d functions building HTTP requests, want exactly 1", path, requestBuilders)
	}
}

// TestNoJavaScriptRuntime asserts the JS interpreter stays gone. A watch page's
// adaptive formats carry no signatureCipher, so there is nothing to run, and a
// dependency on a JS runtime is a large attack surface for a feature we do not
// have.
//
// Vendoring one is the obvious way in and shelling out to one is the quiet way,
// so both are checked. A tool that spawns node to undo a cipher has the same
// dependency as one that links a VM, it just does not say so in go.mod.
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

	interpreters := map[string]bool{"node": true, "nodejs": true, "deno": true, "bun": true, "phantomjs": true, "qjs": true}
	for _, path := range repoFiles(t) {
		if filepath.Base(path) == "policy_test.go" {
			continue
		}
		for lit, pos := range stringLiterals(t, path) {
			if interpreters[strings.ToLower(lit)] {
				t.Errorf("%s: names %q, which is a JavaScript runtime to shell out to. "+
					"Nothing here runs JS, and the day something needs to it is a design decision rather than a patch.", pos, lit)
			}
		}
	}
}

// execAllowed is every file permitted to start a process, with why.
//
// Doc 06 section 5 says exec.Command lives in one file. It lives in three, and
// the deviation is deliberate rather than drift: the spec was written before
// --use-yt-dlp and config edit existed, and both of them are the user asking for
// another program by name. The rule that still matters is that a subprocess is
// something the user chose, never something a parser reaches for on its own, and
// that is what the allowlist encodes. Doc 06 records the same three.
var execAllowed = map[string]string{
	"youtube/ffmpeg.go": "muxes a video-only and an audio-only file into one, which is the whole reason " +
		"the adaptive formats are worth downloading separately",
	"cli/download.go": "runs yt-dlp, and only behind --use-yt-dlp, which is the user naming the program",
	"cli/config.go":   "opens $EDITOR on the config file, which is the user's own editor on the user's own file",
}

// TestExecIsAccountedFor asserts nothing new starts a process without saying why.
//
// A subprocess is the one thing in here that escapes every other rule in this
// file: it makes its own requests, sends its own headers, and answers with bytes
// no parser in this package has seen. Three of them is a number somebody can hold
// in their head, and the fourth one is where that stops being true.
func TestExecIsAccountedFor(t *testing.T) {
	found := map[string]bool{}
	for _, path := range repoFiles(t) {
		base := filepath.Base(path)
		if base == "policy_test.go" {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(src), "exec.Command") {
			continue
		}
		rel := strings.TrimPrefix(filepath.ToSlash(path), "../")
		found[rel] = true
		if _, ok := execAllowed[rel]; !ok {
			t.Errorf("%s starts a process. A subprocess makes its own requests and answers with bytes "+
				"nothing here parsed, so add the file to execAllowed with the reason it has to.", rel)
		}
	}
	for rel := range execAllowed {
		if !found[rel] {
			t.Errorf("execAllowed names %s, which no longer runs anything. Drop the entry rather than "+
				"leaving a permission nobody uses.", rel)
		}
	}
}

// TestCallIsTheOnlyInnerTubePost asserts there is one way to reach youtubei.
//
// Four things have to be right on an InnerTube request and none of them is
// visible in the response when they are wrong: the harvested key, the headers and
// the body context agreeing on the client, hl and gl in all four places, and the
// alerts check that turns a 200 into a refusal. Call does all four. A second
// POST helper does whichever of them its author remembered, and the version this
// package shipped for a while pinned Accept-Language to English no matter what
// the config said, which is doc 01 section 1.3 broken with nothing to show for it.
func TestCallIsTheOnlyInnerTubePost(t *testing.T) {
	for _, path := range repoFiles(t) {
		base := filepath.Base(path)
		if strings.HasSuffix(path, "_test.go") || base == "call.go" || base == "download.go" {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, "http.MethodPost") && !strings.Contains(line, "http.Post(") {
				continue
			}
			t.Errorf("%s:%d builds a POST outside call.go: %s\n"+
				"Every InnerTube request goes through Call, which is where the key, the client and the locale are decided.",
				path, i+1, strings.TrimSpace(line))
		}
	}
}

// TestEveryInnerTubeBodyCarriesTheLocale is the body half of the locale rule.
//
// Doc 01 section 1.3: hl and gl go in four places, and the context is the one
// that decides what the renderers say. A response asked for with no context comes
// back in the language of the exit node, so a parser reading a count positionally
// still works and one reading the word next to it does not, and the record ends
// up with a subscriber count from a different country's rounding.
func TestEveryInnerTubeBodyCarriesTheLocale(t *testing.T) {
	for _, s := range Clients() {
		client, ok := s.Context("vi", "VN", "")["client"].(map[string]any)
		if !ok {
			t.Errorf("%s: context has no client object", s.Name)
			continue
		}
		if client["hl"] != "vi" || client["gl"] != "VN" {
			t.Errorf("%s: context says hl=%v gl=%v, and the caller asked for vi/VN", s.Name, client["hl"], client["gl"])
		}
	}

	// And that the one place that builds a body actually asks for it. The check is
	// on the source because the alternative is a live request, and the failure it
	// guards against is somebody inlining a literal "en" here on a quiet afternoon.
	src, err := os.ReadFile("call.go")
	if err != nil {
		t.Fatalf("read call.go: %v", err)
	}
	if !strings.Contains(string(src), "effective.Context(c.hl, c.gl,") {
		t.Error("call.go no longer fills the context from the client's own hl and gl, " +
			"so the language on the wire is not the language the config asked for")
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

// tabSlugsThatWereHardcoded are the tab names this package once matched by
// shipping the params blob for them. One of the two blobs had already gone stale
// by the time it was noticed: @RickAstleyYT's posts tab answers to
// EgVwb3N0c_IGBAoCSgA%3D and the shipped blob was EgVwb3N0c_IGBAoCEgA, so the
// match failed and the tool reported no posts tab on a channel that has one.
var tabSlugsThatWereHardcoded = []string{
	"featured", "videos", "shorts", "streams", "releases", "playlists", "posts", "community", "search",
}

// TestNoHardcodedTabParams asserts no non-test file carries a params blob that
// decodes to a tab name.
//
// A blob is server routing, and YouTube changes it without notice. The tab strip
// on the response states every tab's params, so there is never a reason to write
// one down. The check decodes each string literal rather than pattern matching it,
// because a stale blob does not look stale.
func TestNoHardcodedTabParams(t *testing.T) {
	for _, path := range repoFiles(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		for lit, pos := range stringLiterals(t, path) {
			slug := paramsSlug(lit)
			if slug == "" {
				continue
			}
			for _, known := range tabSlugsThatWereHardcoded {
				if slug != known {
					continue
				}
				t.Errorf("%s: %q is the params blob for the %s tab. "+
					"Read the params off the tab strip with FindTab instead; a written down blob goes stale.",
					pos, lit, slug)
			}
		}
	}
}

// pathWalkersAllowed are the files still allowed to walk a continuation path by
// hand, with why. The one that remains scopes its walk to a section on purpose,
// which the general finder cannot do for it.
//
// comments.go used to be here and is not any more: milestone 8 rebuilt it on
// FindContinuationToken and FindContinuationTokenUnder, so the reply token, the
// sort chip token and the next-page token in the same response are told apart by
// the marker they sit under rather than by a fixed path.
var pathWalkersAllowed = map[string]string{
	"parse.go": "extractCommentContinuationToken is scoped to comment-item-section, because a /next " +
		"response holds the related-videos token as well and the two are not interchangeable",
}

// TestContinuationTokenSearchIsNotPathWalking asserts the token finder is asked
// for by key rather than reimplemented at a call site.
//
// The four shapes are all live at once: a /next response for dQw4w9WgXcQ carries
// continuationEndpoint and button.buttonRenderer.command on the same renderer, and
// channel tabs use continuationItemViewModel with the token two continuationCommand
// levels down. A parser written against one path pages some lists and stops after
// page one on the others, which reads as a short playlist rather than as a bug.
func TestContinuationTokenSearchIsNotPathWalking(t *testing.T) {
	for _, path := range repoFiles(t) {
		base := filepath.Base(path)
		if strings.HasSuffix(path, "_test.go") || base == "continuation.go" {
			continue
		}
		if _, ok := pathWalkersAllowed[base]; ok {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(src), `"continuationEndpoint"`) {
			t.Errorf("%s walks continuationEndpoint by hand. Call FindContinuationToken, "+
				"which knows all four shapes and which markers mean a different list. "+
				"If the walk has to be scoped to a section, add the file to pathWalkersAllowed with the reason.", path)
		}
	}
}

// TestOnlyYtidBuildsTheWireForm asserts nothing hand-builds a VL browseId.
//
// VL in front of a playlist id addresses the browse endpoint and means nothing
// else: YouTube never shows it, and a record keyed VLUU... is a second node in the
// graph for a playlist that already has one. Keeping the concatenation in one
// function is what makes that checkable, and it is also what stops the other half
// of the bug, a doubled VLVL on an id that already had the prefix.
//
// Reading the prefix is fine and stays fine, because responses carry the wire form
// and it has to come off. Only writing it is the rule.
func TestOnlyYtidBuildsTheWireForm(t *testing.T) {
	for _, path := range repoFiles(t) {
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, "pkg/ytid") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			if !strings.Contains(line, `"VL" +`) && !strings.Contains(line, `+ "VL"`) {
				continue
			}
			t.Errorf("%s:%d builds a VL browseId by hand: %s\n"+
				"Call ytid.WireID, which is idempotent, and keep the bare id on the record.",
				path, i+1, strings.TrimSpace(line))
		}
	}
}

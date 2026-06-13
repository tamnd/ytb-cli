package youtube

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// playerCipher resolves googlevideo stream URLs for one base.js player. It holds
// the signature-transform recipe (a list of reverse/swap/splice ops) and the
// source of the nsig throttling function, both extracted from base.js once and
// reused for every format of every video served by that player.
type playerCipher struct {
	url    string
	sigOps []decipherOp
	nFunc  string // JS source of the n-parameter function, or "" if not found
}

// decipherOp is one step of the signature transform.
type decipherOp func([]byte) []byte

func reverseOp(b []byte) []byte {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

func spliceOp(n int) decipherOp {
	return func(b []byte) []byte {
		if n > len(b) {
			n = len(b)
		}
		return b[n:]
	}
}

func swapOp(n int) decipherOp {
	return func(b []byte) []byte {
		if len(b) == 0 {
			return b
		}
		pos := n % len(b)
		b[0], b[pos] = b[pos], b[0]
		return b
	}
}

// The signature-op grammar. base.js declares a small helper object whose three
// methods reverse, splice, or swap a char array, then a transform function that
// calls them in sequence. These patterns locate the object and the call list.
const (
	jsVar      = `[a-zA-Z_\$][a-zA-Z_0-9]*`
	reverseDef = `:function\(a\)\{(?:return )?a\.reverse\(\)\}`
	spliceDef  = `:function\(a,b\)\{a\.splice\(0,b\)\}`
	swapDef    = `:function\(a,b\)\{var c=a\[0\];a\[0\]=a\[b(?:%a\.length)?\];a\[b(?:%a\.length)?\]=c(?:;return a)?\}`
)

var (
	sigObjRe = regexp.MustCompile(fmt.Sprintf(
		`var (%s)=\{((?:(?:%s%s|%s%s|%s%s),?\n?)+)\};`,
		jsVar, jsVar, swapDef, jsVar, spliceDef, jsVar, reverseDef))
	sigFuncRe = regexp.MustCompile(fmt.Sprintf(
		`function(?: %s)?\(a\)\{a=a\.split\(""\);\s*((?:(?:a=)?%s\.%s\(a,\d+\);)+)return a\.join\(""\)\}`,
		jsVar, jsVar, jsVar))
	sigReverseRe = regexp.MustCompile(fmt.Sprintf(`(?m)(?:^|,)(%s)%s`, jsVar, reverseDef))
	sigSpliceRe  = regexp.MustCompile(fmt.Sprintf(`(?m)(?:^|,)(%s)%s`, jsVar, spliceDef))
	sigSwapRe    = regexp.MustCompile(fmt.Sprintf(`(?m)(?:^|,)(%s)%s`, jsVar, swapDef))

	// nsig call sites. The throttling function is reached either through an array
	// (b=arr[0](b)) or directly (b=fn(b)); both are anchored on `.get("n"))&&(`,
	// which also covers the newer `.get(b))` form where b=String.fromCharCode(110).
	nArrayRe  = regexp.MustCompile(`\.get\((?:"n"|[a-zA-Z0-9_$]+)\)\)&&\([a-zA-Z0-9_$]+=([a-zA-Z0-9_$]+)\[(\d+)\]\(`)
	nDirectRe = regexp.MustCompile(`\.get\((?:"n"|[a-zA-Z0-9_$]+)\)\)&&\([a-zA-Z0-9_$]+=([a-zA-Z0-9_$]+)\([a-zA-Z0-9_$]+\)`)

	playerJSRe = regexp.MustCompile(`(/s/player/[0-9a-fA-F]{8,}/[^"\\]+/base\.js)`)
)

// extractPlayerJSURL pulls the base.js player URL out of a watch page. The CLI
// needs it to solve signatures and the n parameter.
func extractPlayerJSURL(html string) string {
	if m := playerJSRe.FindString(html); m != "" {
		return "https://www.youtube.com" + m
	}
	return ""
}

// cipherFor fetches base.js for playerURL once and builds (and caches) its cipher.
func (c *Client) cipherFor(ctx context.Context, playerURL string) (*playerCipher, error) {
	if playerURL == "" {
		return nil, fmt.Errorf("no player URL")
	}
	c.cipherMu.Lock()
	if c.cipherCache == nil {
		c.cipherCache = map[string]*playerCipher{}
	}
	if pc, ok := c.cipherCache[playerURL]; ok {
		c.cipherMu.Unlock()
		return pc, nil
	}
	c.cipherMu.Unlock()

	body, code, err := c.Fetch(ctx, playerURL)
	if err != nil {
		return nil, fmt.Errorf("fetch player js: %w", err)
	}
	if code != 200 || len(body) == 0 {
		return nil, fmt.Errorf("fetch player js: HTTP %d", code)
	}
	pc := &playerCipher{url: playerURL}
	pc.sigOps = parseSigOps(body)
	pc.nFunc = extractNFunction(body)

	c.cipherMu.Lock()
	c.cipherCache[playerURL] = pc
	c.cipherMu.Unlock()
	return pc, nil
}

// parseSigOps reproduces base.js's signature transform as a list of ops.
func parseSigOps(js []byte) []decipherOp {
	obj := sigObjRe.FindSubmatch(js)
	fn := sigFuncRe.FindSubmatch(js)
	if len(obj) < 3 || len(fn) < 2 {
		return nil
	}
	objName, objBody, callList := obj[1], obj[2], fn[1]
	var reverseKey, spliceKey, swapKey string
	if m := sigReverseRe.FindSubmatch(objBody); len(m) > 1 {
		reverseKey = string(m[1])
	}
	if m := sigSpliceRe.FindSubmatch(objBody); len(m) > 1 {
		spliceKey = string(m[1])
	}
	if m := sigSwapRe.FindSubmatch(objBody); len(m) > 1 {
		swapKey = string(m[1])
	}
	callRe, err := regexp.Compile(fmt.Sprintf(`(?:a=)?%s\.(%s|%s|%s)\(a,(\d+)\)`,
		regexp.QuoteMeta(string(objName)),
		regexp.QuoteMeta(reverseKey), regexp.QuoteMeta(spliceKey), regexp.QuoteMeta(swapKey)))
	if err != nil {
		return nil
	}
	var ops []decipherOp
	for _, m := range callRe.FindAllSubmatch(callList, -1) {
		arg, _ := strconv.Atoi(string(m[2]))
		switch string(m[1]) {
		case reverseKey:
			ops = append(ops, reverseOp)
		case spliceKey:
			ops = append(ops, spliceOp(arg))
		case swapKey:
			ops = append(ops, swapOp(arg))
		}
	}
	return ops
}

func (pc *playerCipher) decryptSignature(s string) string {
	b := []byte(s)
	for _, op := range pc.sigOps {
		b = op(b)
	}
	return string(b)
}

// extractNFunction locates the n-parameter function in base.js and returns its
// JS source (an anonymous `function(a){...}`), prepended with any global array
// declarations it references. Returns "" when extraction fails; callers then
// leave n untouched (the stream may be throttled but still downloads).
func extractNFunction(js []byte) string {
	var name string
	if m := nArrayRe.FindSubmatch(js); len(m) == 3 {
		arr, idxStr := string(m[1]), string(m[2])
		idx, _ := strconv.Atoi(idxStr)
		if elems := varArrayElements(js, arr); idx < len(elems) {
			name = elems[idx]
		}
	}
	if name == "" {
		if m := nDirectRe.FindSubmatch(js); len(m) == 2 {
			name = string(m[1])
		}
	}
	if name == "" {
		return ""
	}
	src := extractFuncSource(js, name)
	if src == "" {
		return ""
	}
	return globalArrayFixup(js, src) + src
}

// varArrayElements returns the top-level, comma-separated elements of a
// `var name=[...]` array literal declared in js.
func varArrayElements(js []byte, name string) []string {
	open := strings.Index(string(js), "var "+name+"=[")
	if open < 0 {
		return nil
	}
	open += len("var " + name + "=")
	body := matchBracket(string(js)[open:], '[', ']')
	if body == "" {
		return nil
	}
	inner := body[1 : len(body)-1] // strip [ ]
	return splitTopLevel(inner)
}

// extractFuncSource finds a named function in js and returns it as an anonymous
// `function(args){body}` so it can be assigned to a variable and run.
func extractFuncSource(js []byte, name string) string {
	s := string(js)
	for _, pat := range []string{
		name + "=function",
		"var " + name + "=function",
		"function " + name,
	} {
		i := strings.Index(s, pat)
		if i < 0 {
			continue
		}
		rest := s[i+len(pat):]
		// rest begins at the args "(...)" (the "function" keyword, if any, is gone).
		argOpen := strings.IndexByte(rest, '(')
		if argOpen < 0 {
			continue
		}
		args := matchBracket(rest[argOpen:], '(', ')')
		if args == "" {
			continue
		}
		afterArgs := rest[argOpen+len(args):]
		bodyOpen := strings.IndexByte(afterArgs, '{')
		if bodyOpen < 0 {
			continue
		}
		body := matchBracket(afterArgs[bodyOpen:], '{', '}')
		if body == "" {
			continue
		}
		return "function" + args + body
	}
	return ""
}

var identIndexRe = regexp.MustCompile(`([a-zA-Z_$][a-zA-Z0-9_$]*)\[`)

// globalArrayFixup detects global array variables referenced inside the n
// function body and prepends their `var x=[...]` declarations, matching the
// shape of newer players that hoist their opcode tables out of the function.
func globalArrayFixup(js []byte, funcSrc string) string {
	seen := map[string]bool{}
	var prefix strings.Builder
	bodyStart := strings.IndexByte(funcSrc, '{')
	arg := funcArgName(funcSrc)
	for _, m := range identIndexRe.FindAllStringSubmatch(funcSrc[bodyStart:], -1) {
		id := m[1]
		if id == arg || seen[id] {
			continue
		}
		seen[id] = true
		if decl := varArrayDecl(js, id); decl != "" {
			prefix.WriteString(decl)
			prefix.WriteString(";\n")
		}
	}
	if prefix.Len() == 0 {
		return ""
	}
	return prefix.String()
}

func funcArgName(funcSrc string) string {
	o := strings.IndexByte(funcSrc, '(')
	c := strings.IndexByte(funcSrc, ')')
	if o < 0 || c < 0 || c < o {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(funcSrc[o+1:c], ",", 2)[0])
}

// varArrayDecl returns the full `var name=[...]` declaration from js, or "".
func varArrayDecl(js []byte, name string) string {
	open := strings.Index(string(js), "var "+name+"=[")
	if open < 0 {
		return ""
	}
	arr := matchBracket(string(js)[open+len("var "+name+"="):], '[', ']')
	if arr == "" {
		return ""
	}
	return "var " + name + "=" + arr
}

// matchBracket returns s[0:n] spanning a balanced open/close pair starting at
// s[0]==open, honoring string and regex literals, or "" on imbalance.
func matchBracket(s string, open, close byte) string {
	if len(s) == 0 || s[0] != open {
		return ""
	}
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'', '`':
			quote = ch
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}

// splitTopLevel splits a comma-separated list, ignoring commas nested in
// brackets/braces/parens or string literals.
func splitTopLevel(s string) []string {
	var out []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'', '`':
			quote = ch
		case '[', '{', '(':
			depth++
		case ']', '}', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

// decryptNParam runs the extracted n function over the query's `n` value. A
// failure (or an unsolved function) leaves n unchanged rather than erroring.
func (pc *playerCipher) decryptNParam(q url.Values) {
	n := q.Get("n")
	if n == "" || pc.nFunc == "" {
		return
	}
	out, err := evalNFunction(pc.nFunc, n)
	if err != nil || out == "" || out == n || strings.HasPrefix(out, "enhanced_except") {
		return
	}
	q.Set("n", out)
}

// evalNFunction runs jsFunc(arg) in a fresh goja VM (pure Go, no cgo).
func evalNFunction(jsFunc, arg string) (string, error) {
	vm := goja.New()
	if _, err := vm.RunString("var __n=" + jsFunc); err != nil {
		return "", err
	}
	var fn func(string) string
	if err := vm.ExportTo(vm.Get("__n"), &fn); err != nil {
		return "", err
	}
	var out string
	func() {
		defer func() { _ = recover() }() // a broken extraction can panic in goja
		out = fn(arg)
	}()
	return out, nil
}

// resolveURL turns a stream's url/signatureCipher into a final fetchable URL,
// applying the signature transform (if ciphered) and the n-parameter transform.
func (pc *playerCipher) resolveURL(s *Stream) (string, error) {
	raw := s.url
	if raw == "" && s.signatureCipher != "" {
		params, err := url.ParseQuery(s.signatureCipher)
		if err != nil {
			return "", err
		}
		raw = params.Get("url")
		sig := pc.decryptSignature(params.Get("s"))
		sp := params.Get("sp")
		if sp == "" {
			sp = "signature"
		}
		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set(sp, sig)
		pc.decryptNParam(q)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}
	if raw == "" {
		return "", fmt.Errorf("format %d has no url or cipher", s.ITag)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	pc.decryptNParam(q)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/ytb"
)

// surfaces_test.go holds the promise of doc 06 section 4 to the one thing that
// can enforce it: every read the binary offers is a read `ytb serve` and `ytb
// mcp` offer too, from a single registration.
//
// It is a test rather than a rule because the two halves are written in two
// packages. The commands are hand-written in cli/ so a table can be laid out;
// the operations are registered in ytb/ so a host that never links cli/ has
// them. Nothing but this pairs the two, and the failure it catches is silent:
// the command works, the tests pass, and only somebody curling the server finds
// out that half the tool is not there.

// notServed is every command that has no operation, with the reason. A read
// missing from here and from the op table fails the test, so the way to leave
// one out is to write down why.
var notServed = map[string]string{
	"download": "writes a file and takes minutes; a GET that streams a video to disk on the server is not a read",
	"extract":  "shells out to yt-dlp and writes a file, same as download",
	"rdf":      "the edges read in another serialization, and the op table has one op per read rather than one per writer; `ytb edges` is the op and n-triples is a rendering of it",
	"crawl":    "writes the local store, which is this machine's and not YouTube's",
	"archive":  "writes a directory of raw payloads",
	"db":       "manages the local store: stats, vacuum, reset",
	"cache":    "inspects and deletes this machine's response cache; a route that emptied the server's own cache would be a way for a caller to make every later request slow",
	"query":    "runs SQL over the local store",
	"export":   "renders the local store as Markdown into a directory",
	"config":   "shows and writes this machine's config file",
	"auth":     "stores and removes this machine's cookies; a route that took them as a query parameter would be a way to write them into a log",
	"version":  "prints the binary's own version, which the server already states",
}

func TestEveryReadIsServed(t *testing.T) {
	ops := opKeys(t)

	for _, name := range commandVerbs() {
		_, served := ops[name]
		reason, excused := excuse(name)
		switch {
		case served && excused:
			t.Errorf("%q is both served and excused as %q: delete the excuse", name, reason)
		case !served && !excused:
			t.Errorf("`ytb %s` reads and no operation serves it, so it is missing from ytb serve and ytb mcp.\n"+
				"Register it in ytb/ops.go with NoCLI set, or add it to notServed with the reason.", name)
		}
	}
}

// TestNoExcuseOutlivesItsCommand keeps the list above honest. An entry for a
// command that no longer exists is a note about a decision nobody can check.
func TestNoExcuseOutlivesItsCommand(t *testing.T) {
	have := map[string]bool{}
	for _, name := range commandVerbs() {
		have[name] = true
		have[verb(name)] = true // a group is excused whole, so "db" counts
	}
	for name, reason := range notServed {
		if !have[name] {
			t.Errorf("notServed excuses %q (%s) and there is no such command", name, reason)
		}
	}
}

// TestHandWrittenCommandsKeepTheirCommandLine checks the other direction. An op
// that pairs with a hand-written command must carry NoCLI, or kit generates a
// second subcommand under the same name and the reflected one wins.
func TestHandWrittenCommandsKeepTheirCommandLine(t *testing.T) {
	written := map[string]bool{}
	for _, name := range commandVerbs() {
		written[name] = true
	}
	for _, op := range appOps(t) {
		m := op.Meta()
		key := m.Name
		if m.Parent != "" {
			key = m.Parent + " " + m.Name
		}
		if written[key] && !m.NoCLI {
			t.Errorf("op %q would shadow the hand-written `ytb %s`: set NoCLI on it", key, key)
		}
	}
}

// TestEveryOpIsARead is the policy half of the op table, and it is about what
// `ytb serve` and `ytb mcp` expose rather than about help text.
//
// kit gates a Write op differently and annotates it on every surface, and an MCP
// client is entitled to read that annotation and decide a tool is safe to call
// unattended. Nothing in this tool changes anything on YouTube, so every op is a
// read and the day one is not, it is a decision somebody makes on purpose here
// rather than a flag that got copied along with the line above it. Doc 06
// section 5, and ytb/ops.go registers all of them in the read group.
func TestEveryOpIsARead(t *testing.T) {
	for key, m := range opKeys(t) {
		if m.Write {
			t.Errorf("op %q is marked Write, and this tool only reads YouTube. "+
				"If it really writes something, it needs a spec section before it needs a flag.", key)
		}
		if m.Group != "read" {
			t.Errorf("op %q is in group %q, want read. The group is what help, OpenAPI and the MCP "+
				"tool list sort by, and a second group splits the table for no reason.", key, m.Group)
		}
		if m.Summary == "" {
			t.Errorf("op %q has no summary, so it is a nameless tool in the MCP list", key)
		}
	}
}

// TestRoutesAreUniqueAndAddressable is the routes half of the invariant tests.
//
// kit derives three names from one OpMeta: the command path, the HTTP route and
// the MCP tool name, by joining the parent and the name with a space, a slash and
// an underscore. Two ops that collide in any of the three do not fail to
// register, they shadow each other, and which one answers depends on the order
// they were registered in. That is a bug that shows up as a route returning
// somebody else's record.
func TestRoutesAreUniqueAndAddressable(t *testing.T) {
	routes := map[string]string{}
	tools := map[string]string{}
	for key, m := range opKeys(t) {
		route := m.Name
		tool := m.Name
		if m.Parent != "" {
			route = m.Parent + "/" + m.Name
			tool = m.Parent + "_" + m.Name
		}
		if prev, ok := routes[route]; ok {
			t.Errorf("ops %q and %q both answer at /%s, and the second one registered wins", prev, key, route)
		}
		routes[route] = key
		if prev, ok := tools[tool]; ok {
			t.Errorf("ops %q and %q are both the MCP tool %s", prev, key, tool)
		}
		tools[tool] = key

		// A route is a URL path segment, so what is legal in it is narrower than
		// what is legal in a Go string.
		if strings.ToLower(route) != route || strings.ContainsAny(route, " _") {
			t.Errorf("op %q routes to /%s, which is not a path segment: lower case and no spaces or underscores", key, route)
		}
		for _, arg := range m.Args {
			if arg.Name == "" || arg.Help == "" {
				t.Errorf("op %q has an argument with no name or no help, which is a blank field in the OpenAPI schema", key)
			}
		}
	}
	if len(routes) < 20 {
		t.Errorf("only %d routes, and the op table is bigger than that; the walk is wrong", len(routes))
	}
}

// commandVerbs is every verb the escape hatches add, with a group's children
// spelled out the way the op table keys them: "music album", not "music".
func commandVerbs() []string {
	var out []string
	for _, cmd := range escapeHatches() {
		name := verb(cmd.Use)
		if len(cmd.Sub) == 0 {
			out = append(out, name)
			continue
		}
		for _, sub := range cmd.Sub {
			out = append(out, name+" "+verb(sub.Use))
		}
	}
	return out
}

// excuse looks up why a verb has no operation, trying the whole verb and then
// the group it sits in, because a group with no reads is excused whole rather
// than one subcommand at a time.
func excuse(name string) (string, bool) {
	if reason, ok := notServed[name]; ok {
		return reason, true
	}
	reason, ok := notServed[verb(name)]
	return reason, ok
}

// verb is the command word out of a Use line: "music album <id|url>" is "music
// album" to cobra and "album" here, because the caller adds the parent.
func verb(use string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(use), " ")
	return name
}

func opKeys(t *testing.T) map[string]kit.OpMeta {
	t.Helper()
	out := map[string]kit.OpMeta{}
	for _, op := range appOps(t) {
		m := op.Meta()
		key := m.Name
		if m.Parent != "" {
			key = m.Parent + " " + m.Name
		}
		out[key] = m
	}
	return out
}

func appOps(t *testing.T) []kit.Operation {
	t.Helper()
	app := kit.New(kit.Identity{Binary: "ytb", Short: "test"})
	(ytb.Domain{}).Register(app)
	ops := app.Ops()
	if len(ops) == 0 {
		t.Fatal("the domain registered no operations")
	}
	return ops
}

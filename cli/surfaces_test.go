package cli

import (
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

// surfaces_test.go holds the promise of doc 06 section 4 to the one thing that
// can enforce it: every read the binary offers is a read `ytb serve` and `ytb
// mcp` offer too, from a single registration.
//
// It is a test rather than a rule because the two halves are written in two
// packages. The commands are hand-written in cli/ so a table can be laid out;
// the operations are registered in youtube/ so a host that never links cli/ has
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
				"Register it in youtube/ops.go with NoCLI set, or add it to notServed with the reason.", name)
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
	(youtube.Domain{}).Register(app)
	ops := app.Ops()
	if len(ops) == 0 {
		t.Fatal("the domain registered no operations")
	}
	return ops
}

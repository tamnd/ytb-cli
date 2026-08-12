package ytb

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// TestEveryOpBindsItsInput invokes every registered operation once, with no
// network behind it, and asks only that it come back.
//
// The bug it exists for is quiet. kit fills an input by walking the struct's
// declared fields, so an input that reaches for its client through an embedded
// struct compiles, registers, serves, and hands the handler a nil client. The
// first request panics the server. Nothing before this ran a registered op at
// all: the youtube tests call the client directly and the cli tests build rows,
// so the whole registration path was exercised only by curl.
//
// The transport refuses every request, so a handler that reads gets an error and
// a handler that reads nothing (predicates) gets its records. Either is a pass.
// The failure this catches is a panic, which fails the test with the op's name
// on it.
func TestEveryOpBindsItsInput(t *testing.T) {
	app := kit.New(kit.Identity{Binary: "ytb", Short: "test"})
	Domain{}.Register(app)

	// No retries and no delay: a refused request should come straight back
	// rather than be tried four more times with a backoff between them.
	cfg := DefaultConfig()
	cfg.Retries = 0
	cfg.Delay = 0
	c := NewClient(cfg)
	c.http = &http.Client{Transport: refusingTransport{}}

	// The two reads that make no request. Everything else has to fail here,
	// because there is nothing behind it to answer.
	offline := map[string]bool{"id": true, "predicates": true}

	for _, op := range app.Ops() {
		m := op.Meta()
		name := m.Name
		if m.Parent != "" {
			name = m.Parent + " " + m.Name
		}
		t.Run(name, func(t *testing.T) {
			in := kit.Input{Args: []string{"dQw4w9WgXcQ"}, Flags: map[string]any{}}
			rt := kit.RunContext{Client: c}
			err := op.Invoke(t.Context(), in, rt, discardSink{})
			if offline[name] {
				if err != nil {
					t.Fatalf("%s makes no request and still failed: %v", name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("%s answered with no network behind it, which no read can do", name)
			}
			if strings.Contains(err.Error(), "nil pointer") {
				t.Fatalf("%s: %v", name, err)
			}
		})
	}
}

// TestEveryArgIsNamedTheSameEverywhere pairs the arguments an op declares with
// the fields its input binds.
//
// They are written twice: OpMeta.Args names them for the usage line and the
// help, and the input struct names them again for the binding. The command line
// binds by position and never notices a disagreement, but HTTP and MCP pass an
// argument by name, so `ref` in the meta and `Refs` in the struct means
// /v1/video?ref=... sets nothing and answers 200 with an empty body. That is
// how the form written down in the spec went a whole milestone without working.
func TestEveryArgIsNamedTheSameEverywhere(t *testing.T) {
	app := kit.New(kit.Identity{Binary: "ytb", Short: "test"})
	Domain{}.Register(app)

	for _, op := range app.Ops() {
		m := op.Meta()
		name := m.Name
		if m.Parent != "" {
			name = m.Parent + " " + m.Name
		}

		var bound []string
		for _, p := range op.Params() {
			if p.Kind == kit.KindArg {
				bound = append(bound, p.Name)
			}
		}
		if len(m.Args) != len(bound) {
			t.Errorf("%s declares %d arguments and binds %d: %v vs %v", name, len(m.Args), len(bound), argNames(m.Args), bound)
			continue
		}
		for i, arg := range m.Args {
			if arg.Name != bound[i] {
				t.Errorf("%s argument %d is %q in the meta and %q on the input, so passing it by name does nothing.\n"+
					"Add name=%s to the kit tag, or rename the argument in the meta.", name, i, arg.Name, bound[i], arg.Name)
			}
		}
	}
}

func argNames(args []kit.Arg) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a.Name
	}
	return out
}

type discardSink struct{}

func (discardSink) Emit(any) error { return nil }
func (discardSink) Flush() error   { return nil }

// refusingTransport fails every request without opening a socket, so this test
// exercises the binding and never the site.
type refusingTransport struct{}

func (refusingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no network in this test")
}

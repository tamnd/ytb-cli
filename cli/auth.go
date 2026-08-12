package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/youtube"
)

// auth.go is tier 1: the cookies your browser already has, copied over.
//
// Everything else in ytb reads YouTube signed out, and that is the product.
// A session adds the five things doc 01 section 12 names and nothing else, and
// it is worth being clear about what it is not: ytb never asks for a password,
// never drives a login form, never touches a consent screen and never sends a
// cookie anywhere except youtube.com. It cannot post, like, subscribe or delete,
// because nothing in this binary writes to YouTube at all.
//
// The three commands are here rather than in the op table on purpose, and the
// reason is in cli/surfaces_test.go: `ytb serve` would otherwise offer a route
// that takes your cookies as a query parameter, and a query parameter is a thing
// that ends up in a log.

func newAuthCmd() kit.Command {
	return kit.Command{
		Use:   "auth",
		Short: "Manage your YouTube session",
		Sub: []kit.Command{
			newAuthImportCmd(),
			newAuthStatusCmd(),
			newAuthClearCmd(),
		},
	}
}

func newAuthImportCmd() kit.Command {
	var cookies string
	return kit.Command{
		Use:   "import",
		Short: "Store the cookies that make tier 1 work",
		Long: "import reads the cookies out of a browser you are already signed into. It takes a " +
			"Netscape cookies.txt as a path, a Cookie header pasted from the network panel, or " +
			"either of those on stdin with -.\n\n" +
			"Only the session cookies are kept and the rest of the file is dropped. They are " +
			"written 0600 in the data directory, they go into a request header and nowhere " +
			"else, and `ytb auth status` prints their names without their values.",
		Args:  kit.NoArgs,
		Write: true,
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&cookies, "cookies", "", "a cookies.txt path, a Cookie header, or - for stdin")
		},
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			if strings.TrimSpace(cookies) == "" {
				return usageErr("--cookies takes a cookies.txt path, a pasted Cookie header, or - for stdin")
			}
			s, err := youtube.ReadSession(cookies)
			if err != nil {
				return err
			}
			if err := youtube.SaveSession(app.DataDir, s); err != nil {
				return err
			}
			if err := app.Out.Emit(sessionRow(s.Status(youtube.SessionPath(app.DataDir)))); err != nil {
				return err
			}
			return app.Out.Flush()
		},
	}
}

func newAuthStatusCmd() kit.Command {
	return kit.Command{
		Use:   "status",
		Short: "Say which cookies are stored and what they unlock",
		Long: "status names the cookies and never prints one. A value in a terminal is a value in a " +
			"scrollback buffer, and this command exists to be run in front of other people.",
		Args: kit.NoArgs,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			path := youtube.SessionPath(app.DataDir)
			s, err := youtube.LoadSession(app.DataDir)
			if err != nil {
				// A file that will not parse is the one case the client swallows,
				// so this is where it gets said out loud.
				return fmt.Errorf("%w\nrun `ytb auth clear` to remove it and import again", err)
			}
			if err := app.Out.Emit(sessionRow(s.Status(path))); err != nil {
				return err
			}
			return app.Out.Flush()
		},
	}
}

func newAuthClearCmd() kit.Command {
	return kit.Command{
		Use:     "clear",
		Aliases: []string{"logout"},
		Short:   "Forget the stored session",
		Long: "clear deletes the local cookie file. It does not sign the browser out and does not tell " +
			"YouTube anything, because it never talks to YouTube at all.",
		Args:  kit.NoArgs,
		Write: true,
		Run: func(ctx context.Context, _ []string) error {
			app := appFromCtx(ctx)
			if err := youtube.ClearSession(app.DataDir); err != nil {
				return err
			}
			var gone youtube.Session
			if err := app.Out.Emit(sessionRow(gone.Status(youtube.SessionPath(app.DataDir)))); err != nil {
				return err
			}
			return app.Out.Flush()
		},
	}
}

// sessionRow renders the status. The columns are names, a count and a tier, and
// there is no branch of this function that can reach a cookie value: the row is
// built from a SessionStatus, which does not have one to give.
func sessionRow(st youtube.SessionStatus) Row {
	return Row{
		Cols: []string{"present", "tier", "cookies", "missing", "source", "imported"},
		Vals: []string{
			boola(st.Present),
			itoa(st.Tier),
			strings.Join(st.Cookies, ", "),
			strings.Join(st.Missing, ", "),
			st.Source,
			importedText(st),
		},
		Value: st,
	}
}

func importedText(st youtube.SessionStatus) string {
	if st.ImportedAt.IsZero() {
		return ""
	}
	return st.ImportedAt.Format("2006-01-02 15:04")
}

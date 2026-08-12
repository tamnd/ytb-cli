package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/ytb"
)

// newArchiveCmd is doc 04 section 4.1: one read written down in full.
func newArchiveCmd() kit.Command {
	var (
		dir      string
		items    int
		comments int
		captions bool
	)
	return kit.Command{
		Use:   "archive <ref>",
		Short: "Write one read down in full: page, payloads, headers, and what ytb parsed",
		Long: `Fetch a video, channel, or playlist for real and write everything about the read
into a directory: page.html, innertube/<endpoint>-<client>.json for each payload,
meta.json with the request headers as they were sent, and record.json with what
ytb parsed out of them.

Any header that carried a session is replaced with a note, so an archive is safe
to attach to a bug report.

It never answers from the cache, because the point is what YouTube is serving
now. It does write what it fetched into the cache, which is why it will not run
with --no-cache.

A page ytb cannot parse still archives, with the parse error in record.json
beside the bytes. That is the case this command is for.`,
		Args: kit.ExactArgs(1),
		Flags: func(f *kit.FlagSet) {
			f.StringVar(&dir, "dir", "", "directory to write into (default <data-dir>/archive/<ref>-<unix>)")
			f.IntVar(&items, "items", 0, "playlist items to read (0 = header only)")
			f.IntVar(&comments, "comments", 0, "comments to read (0 = none)")
			f.BoolVar(&captions, "captions", false, "read the caption list too")
		},
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			ref := args[0]
			if dir == "" {
				dir = filepath.Join(app.DataDir, "archive", fmt.Sprintf("%s-%d", safeName(ref), time.Now().Unix()))
			}
			if app.dryRun {
				app.logf("would archive %s into %s", ref, dir)
				return nil
			}
			cap, err := ytb.Archive(ctx, app.Client, ref, dir, ytb.ClaimOptions{
				Items:     items,
				Comments:  comments,
				Captions:  captions,
				Microdata: true,
			})
			if err != nil {
				return err
			}
			sort.Strings(cap.Files)
			if err := app.Out.Emit(Row{
				Cols: []string{"dir", "files", "reads", "bytes", "parse_error"},
				Vals: []string{
					cap.Dir, itoa(len(cap.Files)), itoa(cap.Reads),
					humanBytes(int64(cap.Bytes)), oneline(cap.ParseError),
				},
				Value: cap,
			}); err != nil {
				return err
			}
			return app.Out.Flush()
		},
	}
}

// safeName turns a reference into something that can be a directory name. A
// watch URL and a bare id then archive into different directories, which is
// right: they are two ways of asking and the point of an archive is the asking.
func safeName(ref string) string {
	out := make([]rune, 0, len(ref))
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '.')
		}
	}
	s := string(out)
	if len(s) > 60 {
		s = s[:60]
	}
	if s == "" {
		s = "archive"
	}
	return s
}

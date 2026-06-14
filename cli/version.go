package cli

import (
	"context"
	"fmt"
	"runtime"

	"github.com/tamnd/any-cli/kit"
)

func newVersionCmd() kit.Command {
	var short bool
	return kit.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  kit.NoArgs,
		Flags: func(f *kit.FlagSet) {
			f.BoolVar(&short, "short", false, "print just the version number")
		},
		Run: func(_ context.Context, _ []string) error {
			if short {
				_, _ = fmt.Fprintln(cmdOut, Version)
				return nil
			}
			_, _ = fmt.Fprintf(cmdOut, "ytb %s (commit %s, built %s, %s/%s, %s)\n",
				Version, Commit, Date, runtime.GOOS, runtime.GOARCH, runtime.Version())
			return nil
		},
	}
}

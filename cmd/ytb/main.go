// Command youtube is a single-binary command line for YouTube.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/fang"
	"github.com/tamnd/ytb-cli/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := cli.Root()
	// fang gives us styled help, errors, version and shell completion for free;
	// the command tree and its exit-code mapping live in the cli package.
	err := fang.Execute(ctx, root,
		fang.WithVersion(cli.Version),
		fang.WithNotifySignal(os.Interrupt, syscall.SIGTERM),
	)
	os.Exit(cli.ExitCode(err))
}

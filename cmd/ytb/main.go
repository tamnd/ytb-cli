// Command ytb is a single-binary command line for YouTube.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(kit.Run(ctx, cli.NewApp()))
}

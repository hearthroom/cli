// Command hearthroom is the Hearthroom command-line client.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/hearthroom/cli/internal/cli"
)

// Set by GoReleaser through -ldflags; defaults describe a source build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.Main(ctx, cli.BuildInfo{Version: version, Commit: commit, Date: date}, os.Args[1:]))
}

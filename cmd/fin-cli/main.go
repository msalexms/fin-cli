// Command fin-cli is a terminal financial monitor by AMSTech: an interactive
// watchlist dashboard plus one-shot commands for quotes and scripting.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/msalexms/fin-cli/internal/cli"
	"github.com/msalexms/fin-cli/internal/tui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := cli.NewRoot(tui.Run).ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fin-cli:", err)
		os.Exit(cli.ExitCode(err))
	}
}

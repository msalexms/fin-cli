// Command fin-cli is a Linux terminal financial monitor.
//
// Usage:
//
//	fin-cli                    → interactive TUI
//	fin-cli quote AAPL         → one-shot snapshot
//	fin-cli quote US0378331005 → autodetected ISIN
//	fin-cli quote --isin <id>  → explicit ISIN
//	fin-cli add  AAPL          → append to watchlist (validates online)
//	fin-cli list               → print watchlist
//	fin-cli remove AAPL        → drop from watchlist
//	fin-cli config edit        → open config.toml in $EDITOR
//	fin-cli purge              → clear on-disk caches
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"fin-cli/internal/cli"
	"fin-cli/internal/tui"
	"fin-cli/internal/version"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	root := newRootCmd()
	root.SetContext(ctx)

	if err := root.Execute(); err != nil {
		// Cobra prints the error itself on RunE failures; add a minimal fallback.
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(cli.ExitGeneric)
	}
}

func newRootCmd() *cobra.Command {
	var opt cli.AppOptions

	// appFactory is called lazily by each subcommand so that flags have been
	// parsed before App construction. Called at most once per invocation.
	var app *cli.App
	appFactory := func() (*cli.App, error) {
		if app != nil {
			return app, nil
		}
		a, err := cli.NewApp(opt)
		if err != nil {
			return nil, err
		}
		app = a
		return app, nil
	}

	root := &cobra.Command{
		Use:   "fin-cli",
		Short: "Terminal financial monitor",
		Long: `fin-cli is a minimalist Linux terminal financial monitor.

When run without a subcommand, it opens an interactive TUI dashboard showing
your watchlist with live quotes and a 30-session chart. Use the subcommands
below for one-shot operations.`,
		Version: fmt.Sprintf("%s (commit %s, built %s)", version.Version, version.Commit, version.Date),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := appFactory()
			if err != nil {
				return err
			}
			defer a.Close()
			return tui.Run(cmd.Context(), a)
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&opt.Debug, "debug", false, "enable debug logging to the log file")
	pf.StringVar(&opt.FinnhubKey, "finnhub-key", "", "override Finnhub API key (takes precedence over env/config)")
	pf.StringVar(&opt.OpenFIGIKey, "openfigi-key", "", "override OpenFIGI API key")
	pf.StringVar(&opt.ConfigPath, "config", "", "override path to config.toml")
	pf.StringVar(&opt.WatchlistPath, "watchlist", "", "override path to watchlist.toml")

	root.AddCommand(
		cli.NewQuoteCmd(appFactory),
		cli.NewAddCmd(appFactory),
		cli.NewRemoveCmd(appFactory),
		cli.NewListCmd(appFactory),
		cli.NewConfigCmd(appFactory),
		cli.NewPurgeCmd(appFactory),
	)
	return root
}

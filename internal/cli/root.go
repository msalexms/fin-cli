// Package cli implements fin-cli's Cobra commands and the one-shot quote
// renderer.
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/msalexms/fin-cli/internal/app"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/version"
)

// Exit codes.
const (
	ExitOK       = 0
	ExitGeneric  = 1
	ExitUsage    = 2
	ExitNetwork  = 3
	ExitConfig   = 4
	ExitNotFound = 5
)

// ExitCode maps an error to the documented exit codes.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, domain.ErrInvalidInput):
		return ExitUsage
	case errors.Is(err, domain.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, domain.ErrUnauthorized), errors.Is(err, domain.ErrNoAPIKey):
		return ExitConfig
	case domain.IsTransient(err):
		return ExitNetwork
	default:
		return ExitGeneric
	}
}

// TUIRunner starts the interactive dashboard. It is injected by main so this
// package does not depend on the TUI.
type TUIRunner func(ctx context.Context, a *app.App) error

// NewRoot builds the command tree.
func NewRoot(runTUI TUIRunner) *cobra.Command {
	opts := app.Options{KeyFlags: map[string]string{}}
	var keys struct{ finnhub, twelvedata, alphavantage, openfigi string }

	// withApp builds the App after flags are parsed and closes it afterwards.
	withApp := func(fn func(cmd *cobra.Command, args []string, a *app.App) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			for k, v := range map[string]string{
				"finnhub.api_key": keys.finnhub, "twelvedata.api_key": keys.twelvedata,
				"alphavantage.api_key": keys.alphavantage, "openfigi.api_key": keys.openfigi,
			} {
				if v != "" {
					opts.KeyFlags[k] = v
				}
			}
			a, err := app.New(opts)
			if err != nil {
				return err
			}
			defer a.Close()
			return fn(cmd, args, a)
		}
	}

	root := &cobra.Command{
		Use:   "fin-cli",
		Short: "Terminal financial monitor · an AMSTech project",
		Long: `fin-cli is a terminal financial monitor by AMSTech.

Run it without a subcommand to open the interactive dashboard: your watchlist
with live quotes, sparklines and a one-month chart. Use the subcommands for
one-shot quotes, scripting and watchlist management.

Works out of the box with Yahoo Finance (no key). Add a free Finnhub key for
fundamentals: fin-cli config set finnhub.api_key <KEY>`,
		Version:       version.String(),
		Args:          usageArgs(cobra.NoArgs),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: withApp(func(cmd *cobra.Command, _ []string, a *app.App) error {
			return runTUI(cmd.Context(), a)
		}),
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	})
	root.SetVersionTemplate("fin-cli {{.Version}}\n")

	pf := root.PersistentFlags()
	pf.BoolVar(&opts.Debug, "debug", false, "write a debug log (see `fin-cli config path`)")
	pf.StringVar(&opts.ConfigPath, "config", "", "use this config.toml instead of the default")
	pf.StringVar(&opts.WatchlistPath, "watchlist", "", "use this watchlist.toml instead of the default")
	pf.StringVar(&keys.finnhub, "finnhub-key", "", "Finnhub API key for this run (overrides env and config)")
	pf.StringVar(&keys.twelvedata, "twelvedata-key", "", "Twelve Data API key for this run")
	pf.StringVar(&keys.alphavantage, "alphavantage-key", "", "Alpha Vantage API key for this run")
	pf.StringVar(&keys.openfigi, "openfigi-key", "", "OpenFIGI API key for this run")

	root.AddCommand(
		newQuoteCmd(withApp),
		newAddCmd(withApp),
		newRemoveCmd(withApp),
		newListCmd(withApp),
		newExportCmd(withApp),
		newConfigCmd(withApp),
		newPurgeCmd(withApp),
	)
	return root
}

type appRunner = func(func(*cobra.Command, []string, *app.App) error) func(*cobra.Command, []string) error

// usageArgs marks argument-count errors as usage errors (exit code 2).
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
		return nil
	}
}

func invalidFormat(f string, valid ...string) error {
	return fmt.Errorf("%w: unknown format %q (valid: %v)", domain.ErrInvalidInput, f, valid)
}

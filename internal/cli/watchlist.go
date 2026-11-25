package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/msalexms/fin-cli/internal/app"
	"github.com/msalexms/fin-cli/internal/domain"
	"github.com/msalexms/fin-cli/internal/watchlist"
)

func newAddCmd(withApp appRunner) *cobra.Command {
	var forceISIN, noValidate bool
	cmd := &cobra.Command{
		Use:   "add <ticker|isin>...",
		Short: "Add tickers to the watchlist",
		Long: `Add one or more tickers (or ISINs) to the watchlist. Each one is checked
against the configured providers first, so typos are caught early; use
--no-validate to skip the check when offline.`,
		Example: "  fin-cli add AAPL MSFT SAN.MC\n  fin-cli add US0378331005\n  fin-cli add NVDA --no-validate",
		Args:    usageArgs(cobra.MinimumNArgs(1)),
		RunE: withApp(func(cmd *cobra.Command, args []string, a *app.App) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			var errs []error
			for _, arg := range args {
				t, err := a.ResolveInput(ctx, arg, forceISIN)
				if err == nil && !noValidate {
					_, err = a.Quotes.Get(ctx, t, false)
				}
				if err == nil {
					err = a.Watchlist.Add(t)
				}
				switch {
				case errors.Is(err, watchlist.ErrAlreadyPresent):
					fmt.Fprintf(out, "%s is already in the watchlist\n", t)
				case err != nil:
					errs = append(errs, fmt.Errorf("%s: %w", arg, err))
				case !strings.EqualFold(string(t), arg):
					fmt.Fprintf(out, "added %s (%s)\n", t, arg)
				default:
					fmt.Fprintf(out, "added %s\n", t)
				}
			}
			return errors.Join(errs...)
		}),
	}
	cmd.Flags().BoolVar(&forceISIN, "isin", false, "treat arguments as ISINs")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "do not check the ticker online")
	return cmd
}

func newRemoveCmd(withApp appRunner) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <ticker|isin>...",
		Aliases: []string{"rm"},
		Short:   "Remove tickers from the watchlist",
		Args:    usageArgs(cobra.MinimumNArgs(1)),
		RunE: withApp(func(cmd *cobra.Command, args []string, a *app.App) error {
			var errs []error
			for _, arg := range args {
				t, err := a.ResolveInput(cmd.Context(), arg, false)
				if err == nil {
					err = a.Watchlist.Remove(t)
				}
				if errors.Is(err, watchlist.ErrNotPresent) {
					err = fmt.Errorf("%w: %s is not in the watchlist", domain.ErrNotFound, t)
				}
				if err != nil {
					errs = append(errs, err)
					continue
				}
				fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", t)
			}
			return errors.Join(errs...)
		}),
	}
}

func newListCmd(withApp appRunner) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Print the watchlist",
		Args:    usageArgs(cobra.NoArgs),
		RunE: withApp(func(cmd *cobra.Command, _ []string, a *app.App) error {
			ts, err := a.Watchlist.Load()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			switch format {
			case "text":
				if len(ts) == 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "watchlist is empty · add tickers with: fin-cli add AAPL")
				}
				for _, t := range ts {
					fmt.Fprintln(out, t)
				}
				return nil
			case "json":
				names := make([]string, len(ts))
				for i, t := range ts {
					names[i] = string(t)
				}
				return writeJSON(out, map[string][]string{"tickers": names})
			default:
				return invalidFormat(format, "text", "json")
			}
		}),
	}
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text|json")
	return cmd
}

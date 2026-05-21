package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"fin-cli/internal/domain"
	"fin-cli/internal/watchlist"
)

// NewAddCmd returns `fin-cli add [ticker|isin]`.
func NewAddCmd(appFactory func() (*App, error)) *cobra.Command {
	var (
		isISIN     bool
		noValidate bool
	)
	cmd := &cobra.Command{
		Use:   "add [ticker]",
		Short: "Add a ticker to the watchlist",
		Long: `Add a ticker to the watchlist. The argument is validated against Finnhub
before persisting. Pass --no-validate to skip the network check (useful offline).
ISINs are auto-detected by format and resolved via OpenFIGI.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()

			ctx := cmd.Context()
			arg := strings.TrimSpace(args[0])

			ticker, err := app.ResolveInput(ctx, arg, isISIN)
			if err != nil {
				return exitError(err)
			}

			if !noValidate {
				if !app.HasFinnhubKey() {
					return exitError(fmt.Errorf("%w: cannot validate without an api key (use --no-validate to skip)", domain.ErrNoAPIKey))
				}
				if _, err := app.Quotes.Get(ctx, ticker, true); err != nil {
					return exitError(fmt.Errorf("validation failed for %s: %w", ticker, err))
				}
			}

			if err := app.Watchlist.Add(ticker); err != nil {
				if errors.Is(err, watchlist.ErrAlreadyPresent) {
					fmt.Fprintf(os.Stderr, "%s already in watchlist\n", ticker)
					return nil
				}
				return exitError(err)
			}
			fmt.Printf("added %s\n", ticker)
			return nil
		},
	}
	cmd.Flags().BoolVar(&isISIN, "isin", false, "treat argument as an ISIN")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "skip provider validation (offline)")
	return cmd
}

// NewRemoveCmd returns `fin-cli remove [ticker]`.
func NewRemoveCmd(appFactory func() (*App, error)) *cobra.Command {
	return &cobra.Command{
		Use:     "remove [ticker]",
		Aliases: []string{"rm"},
		Short:   "Remove a ticker from the watchlist",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()

			t := domain.Ticker(strings.ToUpper(strings.TrimSpace(args[0])))
			if err := app.Watchlist.Remove(t); err != nil {
				if errors.Is(err, watchlist.ErrNotPresent) {
					return exitError(fmt.Errorf("%w: %s", domain.ErrNotFound, t))
				}
				return exitError(err)
			}
			fmt.Printf("removed %s\n", t)
			return nil
		},
	}
}

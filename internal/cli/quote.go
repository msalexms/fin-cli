package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/msalexms/fin-cli/internal/app"
	"github.com/msalexms/fin-cli/internal/domain"
)

func newQuoteCmd(withApp appRunner) *cobra.Command {
	var (
		forceISIN bool
		format    string
		noChart   bool
	)
	cmd := &cobra.Command{
		Use:   "quote <ticker|isin>",
		Short: "Print a quote snapshot with a one-month chart",
		Long: `Fetch a quote and print a snapshot: price, change, key stats and a chart of
the last ~22 sessions.

Tickers use Yahoo-style suffixes for non-US markets (SAN.MC, VOD.L, 7203.T),
crypto and FX pairs (BTC-USD, EURUSD=X) and indices (^GSPC). ISINs such as
US0378331005 are detected automatically and resolved through OpenFIGI.`,
		Example: `  fin-cli quote AAPL
  fin-cli quote US0378331005
  fin-cli quote BTC-USD --format json`,
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: withApp(func(cmd *cobra.Command, args []string, a *app.App) error {
			if format != "text" && format != "json" {
				return invalidFormat(format, "text", "json")
			}
			ctx := cmd.Context()
			t, err := a.ResolveInput(ctx, args[0], forceISIN)
			if err != nil {
				return err
			}
			q, err := a.Quotes.Get(ctx, t, false)
			if err != nil {
				return fmt.Errorf("%s: %w", t, err)
			}
			var candles []domain.Candle
			if !noChart {
				// The chart is best effort: a quote without history is still useful.
				if candles, err = a.Quotes.History(ctx, t, domain.HistorySessions, false); err != nil {
					a.Logger.Debug("history unavailable", "ticker", t, "err", err)
				}
			}
			out := cmd.OutOrStdout()
			if format == "json" {
				return writeJSON(out, struct {
					Quote   domain.Quote    `json:"quote"`
					Candles []domain.Candle `json:"candles"`
				}{q, candles})
			}
			RenderQuote(out, q, candles, RenderOptions{Printer: a.Printer, Width: termWidth(out)})
			return nil
		}),
	}
	cmd.Flags().BoolVar(&forceISIN, "isin", false, "treat the argument as an ISIN even if it does not look like one")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "output format: text|json")
	cmd.Flags().BoolVar(&noChart, "no-chart", false, "skip the price history (one request less)")
	return cmd
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

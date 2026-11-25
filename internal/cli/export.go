package cli

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/msalexms/fin-cli/internal/app"
	"github.com/msalexms/fin-cli/internal/domain"
)

func newExportCmd(withApp appRunner) *cobra.Command {
	var format, output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export watchlist quotes as CSV or JSON",
		Long: `Export the latest quote of every watchlist ticker. Quotes younger than the
polling interval are served from the cache. Tickers that fail are reported on
stderr and skipped.`,
		Example: "  fin-cli export\n  fin-cli export -f json -o quotes.json",
		Args:    usageArgs(cobra.NoArgs),
		RunE: withApp(func(cmd *cobra.Command, _ []string, a *app.App) error {
			if format != "csv" && format != "json" {
				return invalidFormat(format, "csv", "json")
			}
			ts, err := a.Watchlist.Load()
			if err != nil {
				return err
			}
			if len(ts) == 0 {
				return fmt.Errorf("%w: watchlist is empty", domain.ErrInvalidInput)
			}

			var qs []domain.Quote
			var errs []error
			for _, t := range ts {
				q, err := a.Quotes.Get(cmd.Context(), t, false)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %v\n", t, err)
					errs = append(errs, err)
					continue
				}
				qs = append(qs, q)
			}
			if len(qs) == 0 {
				return fmt.Errorf("no quote could be fetched: %w", errors.Join(errs...))
			}

			var w io.Writer = cmd.OutOrStdout()
			if output != "" {
				f, err := os.Create(output)
				if err != nil {
					return err
				}
				defer f.Close()
				w = f
			}
			if format == "json" {
				return writeJSON(w, map[string][]domain.Quote{"quotes": qs})
			}
			return writeCSV(w, qs)
		}),
	}
	cmd.Flags().StringVarP(&format, "format", "f", "csv", "output format: csv|json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "write to this file instead of stdout")
	return cmd
}

func writeCSV(w io.Writer, qs []domain.Quote) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"symbol", "name", "price", "change", "change_pct", "currency",
		"volume", "market_cap_millions", "exchange", "source", "stale", "as_of"})
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	for _, q := range qs {
		vol, mcap := "", ""
		if q.Volume.Valid {
			vol = strconv.FormatInt(q.Volume.Value, 10)
		}
		if q.MarketCap.Valid {
			mcap = f(q.MarketCap.Value)
		}
		_ = cw.Write([]string{string(q.Symbol), q.Name, f(q.Price), f(q.Change), f(q.ChangePct), q.Currency,
			vol, mcap, q.Exchange, q.Source, strconv.FormatBool(q.Stale), q.AsOf.Format(time.RFC3339)})
	}
	cw.Flush()
	return cw.Error()
}

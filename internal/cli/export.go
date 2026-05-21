package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"fin-cli/internal/domain"
)

// exportRow is the data shape written by `fin-cli export`.
type exportRow struct {
	Symbol    string    `json:"symbol"`
	Name      string    `json:"name"`
	Price     float64   `json:"price"`
	Change    float64   `json:"change"`
	ChangePct float64   `json:"change_pct"`
	Volume    int64     `json:"volume"`
	MarketCap float64   `json:"market_cap"`
	Currency  string    `json:"currency"`
	Exchange  string    `json:"exchange"`
	Source    string    `json:"source"`
	FetchedAt time.Time `json:"fetched_at"`
}

// NewExportCmd returns `fin-cli export`.
func NewExportCmd(appFactory func() (*App, error)) *cobra.Command {
	var (
		format string
		output string
	)
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export watchlist quotes to CSV or JSON",
		Long: `Export the current watchlist with latest quote data.
Uses cached data when fresh; does not force-refresh by default.

Examples:
  fin-cli export                     # CSV to stdout
  fin-cli export --format=json       # JSON to stdout
  fin-cli export --output=quotes.csv # CSV to file`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()

			ts, err := app.Watchlist.Load()
			if err != nil {
				return exitError(err)
			}
			if len(ts) == 0 {
				fmt.Fprintln(os.Stderr, "watchlist is empty; nothing to export")
				return nil
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			// Gather quotes (from cache when available).
			rows := make([]exportRow, 0, len(ts))
			for _, t := range ts {
				q, qerr := app.Quotes.Get(ctx, t, false)
				if qerr != nil {
					fmt.Fprintf(os.Stderr, "warning: %s: %v\n", t, qerr)
					continue
				}
				vol := int64(0)
				if q.Volume.Valid {
					vol = q.Volume.Value
				}
				mcap := 0.0
				if q.MarketCap.Valid {
					mcap = q.MarketCap.Value
				}
				rows = append(rows, exportRow{
					Symbol:    string(q.Symbol),
					Name:      q.Name,
					Price:     q.Price,
					Change:    q.Change,
					ChangePct: q.ChangePct,
					Volume:    vol,
					MarketCap: mcap,
					Currency:  q.Currency,
					Exchange:  q.Exchange,
					Source:    string(q.Source),
					FetchedAt: q.FetchedAt,
				})
			}

			if len(rows) == 0 {
				return exitError(fmt.Errorf("%w: no data available for any ticker", domain.ErrPartialData))
			}

			// Determine output writer.
			w := os.Stdout
			if output != "" {
				f, ferr := os.Create(output)
				if ferr != nil {
					return exitError(ferr)
				}
				defer f.Close()
				w = f
			}

			switch format {
			case "csv", "":
				return writeExportCSV(w, rows)
			case "json":
				return writeExportJSON(w, rows)
			default:
				return exitError(fmt.Errorf("%w: unknown format %q (use csv or json)", domain.ErrInvalidInput, format))
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", "csv", "output format: csv|json")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file (default: stdout)")
	return cmd
}

func writeExportCSV(w *os.File, rows []exportRow) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header
	if err := cw.Write([]string{
		"Symbol", "Name", "Price", "Change", "Change%", "Volume",
		"MarketCap(M)", "Currency", "Exchange", "Source", "FetchedAt",
	}); err != nil {
		return err
	}

	for _, r := range rows {
		rec := []string{
			r.Symbol,
			r.Name,
			fmt.Sprintf("%.4f", r.Price),
			fmt.Sprintf("%.4f", r.Change),
			fmt.Sprintf("%.2f", r.ChangePct),
			fmt.Sprintf("%d", r.Volume),
			fmt.Sprintf("%.2f", r.MarketCap),
			r.Currency,
			r.Exchange,
			r.Source,
			r.FetchedAt.Format(time.RFC3339),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	return cw.Error()
}

func writeExportJSON(w *os.File, rows []exportRow) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"quotes": rows})
}

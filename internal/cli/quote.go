package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"fin-cli/internal/domain"
)

// NewQuoteCmd returns `fin-cli quote [ticker|isin]`.
func NewQuoteCmd(appFactory func() (*App, error)) *cobra.Command {
	var (
		isISIN  bool
		format  string
	)
	cmd := &cobra.Command{
		Use:   "quote [ticker]",
		Short: "Print a one-shot quote snapshot",
		Long: `Fetch a quote and render a neofetch-like snapshot with a 30-session chart.

The argument is treated as a ticker symbol. If it matches the ISIN format
(e.g. US0378331005), or --isin is given, it is resolved to a ticker via OpenFIGI first.`,
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

			if !app.HasFinnhubKey() {
				return exitError(fmt.Errorf("%w: set it via `fin-cli config set finnhub.api_key <KEY>` or FIN_CLI_FINNHUB_KEY", domain.ErrNoAPIKey))
			}

			q, err := app.Quotes.Get(ctx, ticker, false)
			if err != nil {
				return exitError(err)
			}

			candles, herr := app.Quotes.History(ctx, ticker, domain.DefaultRange)
			if herr != nil {
				// History is best-effort; log and continue without candles.
				app.Logger.Debug("history failed", "err", herr)
				candles = nil
			}

			switch format {
			case "", "text":
				RenderQuote(os.Stdout, q, candles, app.RenderOptions())
			case "json":
				if err := encodeJSON(os.Stdout, q, candles); err != nil {
					return exitError(err)
				}
			default:
				return exitError(fmt.Errorf("%w: unknown format %q", domain.ErrInvalidInput, format))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&isISIN, "isin", false, "treat argument as an ISIN (auto-detected when it matches the ISIN format)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	return cmd
}

// exitError bridges a domain-classified error to Cobra: it sets the exit code
// via os.Exit after printing to stderr, because Cobra itself only uses 0/1.
func exitError(err error) error {
	if err == nil {
		return nil
	}
	fmt.Fprintln(os.Stderr, "error: "+err.Error())
	code := ExitCodeFor(err)
	if code == ExitOK {
		return nil
	}
	os.Exit(code)
	return err // unreachable
}

func encodeJSON(w *os.File, q domain.Quote, candles []domain.Candle) error {
	type out struct {
		Quote   domain.Quote    `json:"quote"`
		Candles []domain.Candle `json:"candles,omitempty"`
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out{Quote: q, Candles: candles})
}

package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"fin-cli/internal/domain"
)

// NewListCmd returns `fin-cli list`.
func NewListCmd(appFactory func() (*App, error)) *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List watchlist tickers",
		Args:  cobra.NoArgs,
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
			switch format {
			case "", "text":
				if len(ts) == 0 {
					fmt.Println("(empty)")
					return nil
				}
				for _, t := range ts {
					fmt.Println(t)
				}
			case "json":
				raw := make([]string, 0, len(ts))
				for _, t := range ts {
					raw = append(raw, string(t))
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"tickers": raw})
			default:
				return exitError(fmt.Errorf("%w: unknown format %q", domain.ErrInvalidInput, format))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	return cmd
}

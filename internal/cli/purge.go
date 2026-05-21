package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewPurgeCmd returns `fin-cli purge` which clears disk caches.
func NewPurgeCmd(appFactory func() (*App, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "purge",
		Short: "Clear the on-disk quote and ISIN caches",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()
			if err := app.QuoteStore.Purge(); err != nil {
				return exitError(err)
			}
			if err := app.ISINStore.Purge(); err != nil {
				return exitError(err)
			}
			fmt.Println("caches cleared")
			return nil
		},
	}
}

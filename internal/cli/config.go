package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/msalexms/fin-cli/internal/app"
	"github.com/msalexms/fin-cli/internal/config"
	"github.com/msalexms/fin-cli/internal/domain"
)

func newConfigCmd(withApp appRunner) *cobra.Command {
	root := &cobra.Command{
		Use:   "config",
		Short: "Show or change settings",
		Long:  "Show or change settings stored in config.toml.\n\nKeys: " + strings.Join(config.Keys(), ", "),
	}

	field := func(key string) (config.Field, error) {
		f, ok := config.FieldByKey(key)
		if !ok {
			return f, fmt.Errorf("%w: unknown key %q (valid: %s)", domain.ErrInvalidInput, key, strings.Join(config.Keys(), ", "))
		}
		return f, nil
	}
	set := func(a *app.App, key, value string) error {
		f, err := field(key)
		if err != nil {
			return err
		}
		cfg := a.Config()
		if err := f.Set(&cfg, value); err != nil {
			return fmt.Errorf("%w: %s: %v", domain.ErrInvalidInput, key, err)
		}
		if err := a.UpdateConfig(cfg); err != nil {
			return err
		}
		if src := a.Override(f); src != "" {
			fmt.Fprintf(os.Stderr, "note: %s is currently overridden by %s\n", key, src)
		}
		return nil
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "Show every setting (API keys masked)",
			Args:  usageArgs(cobra.NoArgs),
			RunE: withApp(func(cmd *cobra.Command, _ []string, a *app.App) error {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				cfg := a.Config()
				for _, f := range config.Fields {
					v := f.Get(cfg)
					if f.Secret {
						v = config.Mask(v)
					}
					if v == "" {
						v = "(not set)"
					}
					if src := a.Override(f); src != "" {
						v += "  [overridden by " + src + "]"
					}
					fmt.Fprintf(tw, "%s\t%s\n", f.Key, v)
				}
				return tw.Flush()
			}),
		},
		&cobra.Command{
			Use:   "get <key>",
			Short: "Print one setting (API keys masked)",
			Args:  usageArgs(cobra.ExactArgs(1)),
			RunE: withApp(func(cmd *cobra.Command, args []string, a *app.App) error {
				f, err := field(args[0])
				if err != nil {
					return err
				}
				v := f.Get(a.Config())
				if f.Secret {
					v = config.Mask(v)
				}
				fmt.Fprintln(cmd.OutOrStdout(), v)
				return nil
			}),
		},
		&cobra.Command{
			Use:     "set <key> <value>",
			Short:   "Change a setting",
			Example: "  fin-cli config set finnhub.api_key abc123\n  fin-cli config set polling_interval 2m\n  fin-cli config set providers yahoo,finnhub",
			Args:    usageArgs(cobra.ExactArgs(2)),
			RunE: withApp(func(cmd *cobra.Command, args []string, a *app.App) error {
				if err := set(a, args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s updated\n", args[0])
				return nil
			}),
		},
		&cobra.Command{
			Use:   "unset <key>",
			Short: "Reset a setting to its default (clears API keys)",
			Args:  usageArgs(cobra.ExactArgs(1)),
			RunE: withApp(func(cmd *cobra.Command, args []string, a *app.App) error {
				if err := set(a, args[0], ""); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s reset\n", args[0])
				return nil
			}),
		},
		&cobra.Command{
			Use:   "path",
			Short: "Print where fin-cli keeps its files",
			Args:  usageArgs(cobra.NoArgs),
			RunE: withApp(func(cmd *cobra.Command, _ []string, a *app.App) error {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				p := a.Paths
				fmt.Fprintf(tw, "config\t%s\nwatchlist\t%s\ncache\t%s\nlog\t%s (with --debug)\n",
					p.ConfigFile, p.Watchlist, filepath.Dir(p.QuoteCache), p.LogFile)
				return tw.Flush()
			}),
		},
		&cobra.Command{
			Use:   "edit",
			Short: "Open config.toml in $VISUAL / $EDITOR",
			Args:  usageArgs(cobra.NoArgs),
			RunE: withApp(func(_ *cobra.Command, _ []string, a *app.App) error {
				editor := strings.Fields(firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"), defaultEditor()))
				c := exec.Command(editor[0], append(editor[1:], a.Paths.ConfigFile)...)
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				return c.Run()
			}),
		},
	)
	return root
}

func defaultEditor() string {
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func newPurgeCmd(withApp appRunner) *cobra.Command {
	return &cobra.Command{
		Use:   "purge",
		Short: "Delete cached quotes and ISIN lookups",
		Args:  usageArgs(cobra.NoArgs),
		RunE: withApp(func(cmd *cobra.Command, _ []string, a *app.App) error {
			if err := a.QuoteCache.Purge(); err != nil {
				return err
			}
			if err := a.ISINCache.Purge(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "cache cleared")
			return nil
		}),
	}
}

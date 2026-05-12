package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"fin-cli/internal/config"
	"fin-cli/internal/domain"
)

// NewConfigCmd returns `fin-cli config ...`.
func NewConfigCmd(appFactory func() (*App, error)) *cobra.Command {
	root := &cobra.Command{
		Use:   "config",
		Short: "View or edit fin-cli configuration",
	}

	root.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the path of config.toml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()
			fmt.Println(app.Paths.ConfigFile)
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "edit",
		Short: "Open config.toml in $EDITOR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()
			editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi")
			c := exec.Command(editor, app.Paths.ConfigFile)
			c.Stdin = os.Stdin
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "get [key]",
		Short: "Print a config value (secrets are redacted)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()
			v, err := readConfigKey(app.Config, args[0])
			if err != nil {
				return exitError(err)
			}
			if isSecretKey(args[0]) {
				fmt.Println(redactSecret(v))
			} else {
				fmt.Println(v)
			}
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a config value and persist it",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()
			updated, err := writeConfigKey(app.Config, args[0], args[1])
			if err != nil {
				return exitError(err)
			}
			if err := config.Save(app.Paths.ConfigFile, updated); err != nil {
				return exitError(err)
			}
			fmt.Printf("set %s\n", args[0])
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "unset [key]",
		Short: "Clear a config value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := appFactory()
			if err != nil {
				return err
			}
			defer app.Close()
			updated, err := writeConfigKey(app.Config, args[0], "")
			if err != nil {
				return exitError(err)
			}
			if err := config.Save(app.Paths.ConfigFile, updated); err != nil {
				return exitError(err)
			}
			fmt.Printf("unset %s\n", args[0])
			return nil
		},
	})

	return root
}

// knownConfigKeys enumerates dotted paths supported by get/set/unset.
var knownConfigKeys = map[string]bool{
	"finnhub.api_key":   true,
	"openfigi.api_key":  true,
	"polling_interval":  true,
}

func readConfigKey(c config.Config, key string) (string, error) {
	switch key {
	case "finnhub.api_key":
		return c.Finnhub.APIKey, nil
	case "openfigi.api_key":
		return c.OpenFIGI.APIKey, nil
	case "polling_interval":
		return c.PollingInterval.Std().String(), nil
	}
	return "", fmt.Errorf("%w: unknown key %q (supported: %s)", domain.ErrInvalidInput, key, keysList())
}

func writeConfigKey(c config.Config, key, value string) (config.Config, error) {
	switch key {
	case "finnhub.api_key":
		c.Finnhub.APIKey = value
	case "openfigi.api_key":
		c.OpenFIGI.APIKey = value
	case "polling_interval":
		var d config.Duration
		if value == "" {
			return c, fmt.Errorf("%w: polling_interval cannot be empty", domain.ErrInvalidInput)
		}
		if err := d.UnmarshalText([]byte(value)); err != nil {
			return c, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
		c.PollingInterval = d
	default:
		return c, fmt.Errorf("%w: unknown key %q (supported: %s)", domain.ErrInvalidInput, key, keysList())
	}
	return c, nil
}

func keysList() string {
	ks := make([]string, 0, len(knownConfigKeys))
	for k := range knownConfigKeys {
		ks = append(ks, k)
	}
	return strings.Join(ks, ", ")
}

func isSecretKey(key string) bool {
	return strings.Contains(key, "api_key") || strings.Contains(key, "token") || strings.Contains(key, "secret")
}

func redactSecret(v string) string {
	if v == "" {
		return "(unset)"
	}
	if len(v) <= 4 {
		return "****"
	}
	return "****" + v[len(v)-4:]
}

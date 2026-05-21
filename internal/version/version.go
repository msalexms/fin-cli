// Package version exposes build-time metadata injected via -ldflags.
package version

// These are set by -ldflags "-X fin-cli/internal/version.Version=..." at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// UserAgent returns the User-Agent string used for outgoing HTTP calls.
func UserAgent() string {
	return "fin-cli/" + Version
}

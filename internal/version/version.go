// Package version exposes build metadata injected with -ldflags -X.
package version

// Set at build time, e.g.
//
//	-X github.com/msalexms/fin-cli/internal/version.Version=0.2.0
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// UserAgent is sent on every outgoing HTTP request.
func UserAgent() string {
	return "fin-cli/" + Version + " (+https://amstech.dev)"
}

// String is the --version output.
func String() string {
	return Version + " (commit " + Commit + ", built " + Date + ") · AMSTech"
}

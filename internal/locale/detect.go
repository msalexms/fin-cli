// Package locale picks number formatting and Unicode support from the
// environment (LC_ALL, LC_NUMERIC, LANG, TERM).
package locale

import (
	"os"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Printer is a locale-aware number formatter. The zero value formats as English.
type Printer struct {
	p         *message.Printer
	asciiOnly bool
}

// Detect inspects LC_ALL, LC_NUMERIC and LANG (first non-empty wins).
// An explicit C/POSIX locale or TERM=dumb switches to ASCII-only output;
// an unset locale (common on Windows and macOS GUI terminals) does not.
func Detect() Printer {
	raw := strings.TrimSpace(firstNonEmpty(os.Getenv("LC_ALL"), os.Getenv("LC_NUMERIC"), os.Getenv("LANG")))
	ascii := raw == "C" || raw == "POSIX" || strings.HasPrefix(raw, "C.") && !strings.Contains(strings.ToUpper(raw), "UTF") ||
		os.Getenv("TERM") == "dumb"

	tag := language.English
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		raw = raw[:i]
	}
	if raw != "" && raw != "C" && raw != "POSIX" {
		if t, err := language.Parse(strings.ReplaceAll(raw, "_", "-")); err == nil {
			tag = t
		}
	}
	return Printer{p: message.NewPrinter(tag), asciiOnly: ascii}
}

// ASCIIOnly reports whether output should avoid non-ASCII glyphs.
func (p Printer) ASCIIOnly() bool { return p.asciiOnly }

// Sprintf is a locale-aware fmt.Sprintf.
func (p Printer) Sprintf(format string, a ...any) string {
	if p.p == nil {
		return message.NewPrinter(language.English).Sprintf(format, a...)
	}
	return p.p.Sprintf(format, a...)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

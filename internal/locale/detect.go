// Package locale autodetects the user's locale from environment variables
// and exposes number formatting helpers.
package locale

import (
	"os"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Printer is a locale-aware number formatter.
type Printer struct {
	p         *message.Printer
	tag       language.Tag
	asciiOnly bool
}

// Detect inspects LC_ALL, LC_NUMERIC, LANG (in that order) and returns a Printer.
// Returns an ASCII-only, English-locale printer if detection indicates a POSIX/C locale.
func Detect() Printer {
	raw := firstNonEmpty(os.Getenv("LC_ALL"), os.Getenv("LC_NUMERIC"), os.Getenv("LANG"))
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "C" || raw == "POSIX" || strings.HasPrefix(raw, "C.") {
		return Printer{p: message.NewPrinter(language.English), tag: language.English, asciiOnly: true}
	}
	// Normalize "en_US.UTF-8" -> "en-US".
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.ReplaceAll(raw, "_", "-")
	tag, err := language.Parse(raw)
	if err != nil {
		tag = language.English
	}
	return Printer{p: message.NewPrinter(tag), tag: tag, asciiOnly: false}
}

// Tag returns the detected language tag.
func (p Printer) Tag() language.Tag { return p.tag }

// ASCIIOnly reports whether the environment suggests ASCII-only rendering.
func (p Printer) ASCIIOnly() bool { return p.asciiOnly }

// Sprintf is a locale-aware Sprintf.
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

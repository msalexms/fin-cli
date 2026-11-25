package domain

import (
	"errors"
	"testing"
)

func TestIsISIN(t *testing.T) {
	valid := []string{"US0378331005", "ES0113900J37", "DE0007164600", "IE00B4L5Y983"}
	for _, s := range valid {
		if !IsISIN(s) {
			t.Errorf("IsISIN(%q) = false, want true", s)
		}
	}
	invalid := []string{"US0378331006", "AAPL", "us0378331005", "123456789012", "US03783310AB"}
	for _, s := range invalid {
		if IsISIN(s) {
			t.Errorf("IsISIN(%q) = true, want false", s)
		}
	}
}

func TestParseTicker(t *testing.T) {
	ok := map[string]Ticker{" aapl ": "AAPL", "san.mc": "SAN.MC", "btc-usd": "BTC-USD", "^gspc": "^GSPC", "EURUSD=X": "EURUSD=X"}
	for in, want := range ok {
		got, err := ParseTicker(in)
		if err != nil || got != want {
			t.Errorf("ParseTicker(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", "AA PL", "$AAPL", "AAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := ParseTicker(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ParseTicker(%q) err = %v, want ErrInvalidInput", in, err)
		}
	}
}

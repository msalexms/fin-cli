// Package logging configures slog with a file destination and secret redaction.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

const maxLogSize = 5 * 1024 * 1024 // 5 MiB

// Setup returns a slog.Logger that writes to path if debug is true, otherwise
// a no-op logger. It truncates path when it exceeds 5 MiB at startup.
// The returned io.Closer closes the underlying file (or a no-op in non-debug mode).
func Setup(path string, debug bool) (*slog.Logger, io.Closer, error) {
	if !debug {
		return slog.New(discardHandler{}), noopCloser{}, nil
	}

	if st, err := os.Stat(path); err == nil && st.Size() > maxLogSize {
		_ = os.Truncate(path, 0)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}

	inner := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(&redactHandler{inner: inner}), f, nil
}

// --- redaction ---

var (
	tokenQuery = regexp.MustCompile(`([?&]token=)[^&\s"]+`)
	bearer     = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._-]+`)
	figiHeader = regexp.MustCompile(`(?i)(x-openfigi-apikey[:=]\s*)[A-Za-z0-9._-]+`)
)

func redact(s string) string {
	s = tokenQuery.ReplaceAllString(s, "${1}***")
	s = bearer.ReplaceAllString(s, "${1}***")
	s = figiHeader.ReplaceAllString(s, "${1}***")
	return s
}

type redactHandler struct{ inner slog.Handler }

func (r *redactHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return r.inner.Enabled(ctx, lvl)
}

func (r *redactHandler) Handle(ctx context.Context, rec slog.Record) error {
	newRec := slog.NewRecord(rec.Time, rec.Level, redact(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		newRec.AddAttrs(redactAttr(a))
		return true
	})
	return r.inner.Handle(ctx, newRec)
}

func (r *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return &redactHandler{inner: r.inner.WithAttrs(out)}
}

func (r *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{inner: r.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if looksSensitiveKey(a.Key) {
		return slog.String(a.Key, "***")
	}
	if a.Value.Kind() == slog.KindString {
		return slog.String(a.Key, redact(a.Value.String()))
	}
	return a
}

func looksSensitiveKey(k string) bool {
	k = strings.ToLower(k)
	return strings.Contains(k, "key") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "password")
}

// --- discard handler for non-debug mode ---

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool      { return false }
func (discardHandler) Handle(context.Context, slog.Record) error     { return nil }
func (d discardHandler) WithAttrs([]slog.Attr) slog.Handler          { return d }
func (d discardHandler) WithGroup(string) slog.Handler               { return d }

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

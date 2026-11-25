//go:build !unix && !windows

package fsutil

import "os"

// Platforms without advisory locks fall back to no locking; writes stay atomic.
func lockFile(*os.File) error   { return nil }
func unlockFile(*os.File) error { return nil }

//go:build !darwin && !linux

package tenantdb

import (
	"os"
)

// Fallback: no advisory locking on unsupported platforms.
func platformLockFile(_ *os.File) error   { return nil }
func platformUnlockFile(_ *os.File) error { return nil }

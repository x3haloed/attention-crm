//go:build darwin || linux

package tenantdb

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func platformLockFile(f *os.File) error {
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return ErrTenantDBLocked
		}
		return fmt.Errorf("lock tenant db: %w", err)
	}
	return nil
}

func platformUnlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

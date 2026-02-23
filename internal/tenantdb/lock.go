package tenantdb

import (
	"errors"
	"fmt"
	"os"
)

var ErrTenantDBLocked = errors.New("tenant db is locked by another process")

func acquireTenantLock(dbPath string) (*os.File, error) {
	lockPath := dbPath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open tenant lock: %w", err)
	}
	if err := platformLockFile(lockFile); err != nil {
		lockFile.Close()
		return nil, err
	}
	return lockFile, nil
}

func releaseTenantLock(lockFile *os.File) {
	if lockFile == nil {
		return
	}
	_ = platformUnlockFile(lockFile)
	_ = lockFile.Close()
}

package tenantdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir tenant db parent: %w", err)
	}

	lockFile, err := acquireTenantLock(path)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		releaseTenantLock(lockFile)
		return nil, fmt.Errorf("open tenant db: %w", err)
	}
	if err := applyPragmas(db); err != nil {
		db.Close()
		releaseTenantLock(lockFile)
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		releaseTenantLock(lockFile)
		return nil, err
	}
	return &Store{db: db, lockFile: lockFile}, nil
}

func (s *Store) Close() error {
	if s.lockFile != nil {
		releaseTenantLock(s.lockFile)
		s.lockFile = nil
	}
	return s.db.Close()
}

package tenantdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type OpenOptions struct {
	BackupBeforeMigrate bool
	// BackupDir is a base directory for pre-migration backups. When set,
	// OpenWithOptions will create a per-tenant subdirectory underneath it.
	BackupDir string
}

var preMigrateBackupMu sync.Mutex
var preMigrateBackedUp = map[string]bool{}

func Open(path string) (*Store, error) {
	return OpenWithOptions(path, OpenOptions{})
}

func OpenWithOptions(path string, opts OpenOptions) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir tenant db parent: %w", err)
	}

	lockFile, err := acquireTenantLock(path)
	if err != nil {
		return nil, err
	}

	existedBeforeOpen := fileExists(path)

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

	if opts.BackupBeforeMigrate && existedBeforeOpen {
		if err := maybeBackupBeforeMigrate(db, path, opts); err != nil {
			db.Close()
			releaseTenantLock(lockFile)
			return nil, err
		}
	}

	if err := migrate(db); err != nil {
		db.Close()
		releaseTenantLock(lockFile)
		return nil, err
	}
	return &Store{db: db, lockFile: lockFile}, nil
}

func maybeBackupBeforeMigrate(db *sql.DB, dbPath string, opts OpenOptions) error {
	dbPath = filepath.Clean(dbPath)

	preMigrateBackupMu.Lock()
	already := preMigrateBackedUp[dbPath]
	preMigrateBackupMu.Unlock()
	if already {
		return nil
	}

	backupBase := strings.TrimSpace(opts.BackupDir)
	if backupBase == "" {
		backupBase = filepath.Join(filepath.Dir(dbPath), "backups", "migrations")
	}
	tenantKey := filepath.Base(filepath.Dir(dbPath))
	destDir := filepath.Join(backupBase, tenantKey)
	destPath := filepath.Join(destDir, "tenant.premigrate."+time.Now().UTC().Format("20060102T150405Z")+".db")

	if err := (&Store{db: db}).BackupTo(destPath); err != nil {
		return fmt.Errorf("backup before migrate: %w", err)
	}

	preMigrateBackupMu.Lock()
	preMigrateBackedUp[dbPath] = true
	preMigrateBackupMu.Unlock()
	return nil
}

func (s *Store) Close() error {
	if s.lockFile != nil {
		releaseTenantLock(s.lockFile)
		s.lockFile = nil
	}
	return s.db.Close()
}

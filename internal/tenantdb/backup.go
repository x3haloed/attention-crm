package tenantdb

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) BackupTo(destPath string) error {
	destPath = strings.TrimSpace(destPath)
	if destPath == "" {
		return fmt.Errorf("backup path required")
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}

	if fileExists(destPath) {
		suffix := time.Now().UTC().Format("20060102T150405Z")
		_ = os.Rename(destPath, destPath+".bak."+suffix)
	}

	// SQLite does not allow binding parameters for VACUUM INTO, so we must embed a literal.
	escaped := strings.ReplaceAll(destPath, "'", "''")
	_, err := s.db.Exec(`VACUUM INTO '` + escaped + `'`)
	if err != nil {
		return err
	}
	_ = os.Chmod(destPath, 0o600)
	return nil
}

func RestoreFromBackup(dbPath, backupPath string) error {
	dbPath = strings.TrimSpace(dbPath)
	backupPath = strings.TrimSpace(backupPath)
	if dbPath == "" || backupPath == "" {
		return fmt.Errorf("db path and backup path are required")
	}

	lockFile, err := acquireTenantLock(dbPath)
	if err != nil {
		return err
	}
	defer releaseTenantLock(lockFile)

	if _, err := os.Stat(backupPath); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}

	suffix := time.Now().UTC().Format("20060102T150405Z")
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if !fileExists(p) {
			continue
		}
		_ = os.Rename(p, p+".bak."+suffix)
	}

	tmp := dbPath + ".restore.tmp"
	if err := copyFile(backupPath, tmp, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, dbPath); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

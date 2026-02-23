package app

import (
	"path/filepath"

	"attention-crm/internal/tenantdb"
)

func (s *Server) openTenantDB(dbPath string) (*tenantdb.Store, error) {
	opts := tenantdb.OpenOptions{}
	if s.cfg.BackupBeforeMigrate {
		opts.BackupBeforeMigrate = true
		opts.BackupDir = filepath.Join(s.cfg.DataDir, "backups", "migrations")
	}
	return tenantdb.OpenWithOptions(dbPath, opts)
}

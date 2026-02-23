package control

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Tenant struct {
	Slug       string
	Name       string
	StorageKey string
	DBPath     string
}

type Store struct {
	db      *sql.DB
	dataDir string
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir data dir: %w", err)
	}

	controlPath := filepath.Join(dataDir, "control.sqlite")
	db, err := sql.Open("sqlite", controlPath)
	if err != nil {
		return nil, fmt.Errorf("open control db: %w", err)
	}

	if err := applyPragmas(db); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}

	s := &Store{db: db, dataDir: dataDir}
	if err := s.ensureTenantStorageLayout(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) TenantBySlug(slug string) (Tenant, error) {
	row := s.db.QueryRow(`SELECT slug, name, storage_key, db_path FROM tenants WHERE slug = ?`, slug)
	var t Tenant
	var dbPath string
	if err := row.Scan(&t.Slug, &t.Name, &t.StorageKey, &dbPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tenant{}, ErrTenantNotFound
		}
		return Tenant{}, err
	}
	t.DBPath = s.tenantDBPath(t.StorageKey)
	return t, nil
}

func (s *Store) CreateTenant(slug, name string) (Tenant, error) {
	slug = sanitizeSlug(slug)
	if slug == "" {
		return Tenant{}, errors.New("slug required")
	}

	storageKey, err := newStorageKey()
	if err != nil {
		return Tenant{}, err
	}
	dbPath := s.tenantDBPath(storageKey)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return Tenant{}, fmt.Errorf("mkdir tenant dir: %w", err)
	}

	if _, err := s.db.Exec(`INSERT INTO tenants(slug, name, storage_key, db_path) VALUES(?,?,?,?)`, slug, name, storageKey, dbPath); err != nil {
		return Tenant{}, err
	}

	return Tenant{Slug: slug, Name: name, StorageKey: storageKey, DBPath: dbPath}, nil
}

func (s *Store) TenantCount() (int, error) {
	row := s.db.QueryRow(`SELECT COUNT(*) FROM tenants`)
	var c int
	if err := row.Scan(&c); err != nil {
		return 0, err
	}
	return c, nil
}

func sanitizeSlug(slug string) string {
	slug = strings.TrimSpace(strings.ToLower(slug))
	slug = strings.ReplaceAll(slug, " ", "-")
	out := make([]rune, 0, len(slug))
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			out = append(out, r)
		}
	}
	return strings.Trim(string(out), "-")
}

func (s *Store) tenantDBPath(storageKey string) string {
	return filepath.Join(s.dataDir, "tenants", storageKey, "tenant.db")
}

func newStorageKey() (string, error) {
	var buf [12]byte
	if _, err := io.ReadFull(rand.Reader, buf[:]); err != nil {
		return "", fmt.Errorf("generate storage key: %w", err)
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567" // base32 w/out padding
	out := make([]byte, 0, 20)
	for i := 0; i < len(buf); i += 3 {
		b0 := buf[i]
		b1 := byte(0)
		b2 := byte(0)
		if i+1 < len(buf) {
			b1 = buf[i+1]
		}
		if i+2 < len(buf) {
			b2 = buf[i+2]
		}
		// 24 bits -> 4 chars + 1 extra (we output 5 chars per 3 bytes).
		v := uint32(b0)<<16 | uint32(b1)<<8 | uint32(b2)
		out = append(out,
			alphabet[int((v>>19)&31)],
			alphabet[int((v>>14)&31)],
			alphabet[int((v>>9)&31)],
			alphabet[int((v>>4)&31)],
			alphabet[int((v<<1)&31)],
		)
	}
	// prefix makes it easy to spot in logs.
	return "t_" + string(out[:16]), nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tenants (
  slug TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  storage_key TEXT NOT NULL DEFAULT '',
  db_path TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_storage_key
  ON tenants(storage_key)
  WHERE storage_key != '';
`)
	if err != nil {
		return fmt.Errorf("migrate control: %w", err)
	}
	// Older DBs won't have storage_key; add it.
	_ = execAddColumn(db, "tenants", "storage_key TEXT NOT NULL DEFAULT ''")
	// Ensure the unique index exists even if we added the column via ALTER.
	_, _ = db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenants_storage_key ON tenants(storage_key) WHERE storage_key != ''`)
	return nil
}

func execAddColumn(db *sql.DB, table, columnDef string) error {
	_, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + columnDef)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

func (s *Store) ensureTenantStorageLayout() error {
	if err := os.MkdirAll(filepath.Join(s.dataDir, "tenants"), 0o755); err != nil {
		return fmt.Errorf("mkdir tenants dir: %w", err)
	}

	type row struct {
		slug       string
		name       string
		storageKey string
		dbPath     string
	}
	rows, err := s.db.Query(`SELECT slug, name, storage_key, db_path FROM tenants`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.slug, &r.name, &r.storageKey, &r.dbPath); err != nil {
			return err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range all {
		storageKey := strings.TrimSpace(r.storageKey)
		if storageKey == "" {
			var genErr error
			storageKey, genErr = newStorageKey()
			if genErr != nil {
				return genErr
			}
		}
		newPath := s.tenantDBPath(storageKey)
		if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
			return fmt.Errorf("mkdir tenant dir: %w", err)
		}

		oldPath := strings.TrimSpace(r.dbPath)
		if oldPath != "" && oldPath != newPath {
			oldExists := fileExists(oldPath)
			newExists := fileExists(newPath)
			switch {
			case oldExists && !newExists:
				if err := os.Rename(oldPath, newPath); err != nil {
					return fmt.Errorf("move tenant db: %w", err)
				}
			case oldExists && newExists:
				return fmt.Errorf("tenant %q has both legacy and new DB paths; refusing to overwrite", r.slug)
			}
		}

		if _, err := s.db.Exec(`UPDATE tenants SET storage_key = ?, db_path = ? WHERE slug = ?`, storageKey, newPath, r.slug); err != nil {
			return err
		}
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}
	return nil
}

var ErrTenantNotFound = errors.New("tenant not found")

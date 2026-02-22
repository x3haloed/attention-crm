package control

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Tenant struct {
	Slug   string
	DBPath string
	Name   string
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

	return &Store{db: db, dataDir: dataDir}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) TenantBySlug(slug string) (Tenant, error) {
	row := s.db.QueryRow(`SELECT slug, db_path, name FROM tenants WHERE slug = ?`, slug)
	var t Tenant
	if err := row.Scan(&t.Slug, &t.DBPath, &t.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Tenant{}, ErrTenantNotFound
		}
		return Tenant{}, err
	}
	return t, nil
}

func (s *Store) CreateTenant(slug, name string) (Tenant, error) {
	slug = sanitizeSlug(slug)
	if slug == "" {
		return Tenant{}, errors.New("slug required")
	}

	dbPath := filepath.Join(s.dataDir, "tenants", slug+".sqlite")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return Tenant{}, fmt.Errorf("mkdir tenant dir: %w", err)
	}

	_, err := s.db.Exec(`INSERT INTO tenants(slug, db_path, name) VALUES(?,?,?)`, slug, dbPath, name)
	if err != nil {
		return Tenant{}, err
	}

	return Tenant{Slug: slug, DBPath: dbPath, Name: name}, nil
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

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS tenants (
  slug TEXT PRIMARY KEY,
  db_path TEXT NOT NULL,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
`)
	if err != nil {
		return fmt.Errorf("migrate control: %w", err)
	}
	return nil
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

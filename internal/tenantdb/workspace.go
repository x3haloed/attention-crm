package tenantdb

import (
	"database/sql"
	"errors"
)

func (s *Store) primaryWorkspaceID() (int64, error) {
	row := s.db.QueryRow(`SELECT id FROM workspaces ORDER BY id LIMIT 1`)
	var workspaceID int64
	if err := row.Scan(&workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("workspace not initialized")
		}
		return 0, err
	}
	return workspaceID, nil
}

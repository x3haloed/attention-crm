package tenantdb

import (
	"database/sql"
	"errors"
)

func (s *Store) IsOwner(userID int64) (bool, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return false, err
	}
	row := s.db.QueryRow(`SELECT is_owner FROM memberships WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID)
	var isOwner int
	if err := row.Scan(&isOwner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return isOwner == 1, nil
}

func (s *Store) ListMembers(limit int) ([]Member, error) {
	if limit <= 0 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT u.id, u.email, u.name, m.is_owner, m.created_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = ?
ORDER BY m.is_owner DESC, m.created_at ASC
LIMIT ?
`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Member
	for rows.Next() {
		var m Member
		var isOwner int
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &isOwner, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.IsOwner = isOwner == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

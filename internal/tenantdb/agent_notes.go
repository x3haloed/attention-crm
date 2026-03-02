package tenantdb

import (
	"errors"
	"strings"
	"time"
)

type AgentNote struct {
	ID        int64
	AgentKey  string
	Text      string
	CreatedAt string
}

func (s *Store) AppendAgentNote(agentKey string, text string) (int64, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return 0, errors.New("agent key required")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, errors.New("text required")
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`
INSERT INTO agent_notes(workspace_id, agent_key, text, created_at)
VALUES(?,?,?,?)
`, workspaceID, agentKey, text, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) SearchAgentNotes(agentKey string, query string, limit int) ([]AgentNote, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return nil, errors.New("agent key required")
	}
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	like := "%" + query + "%"
	if query == "" {
		like = "%"
	}

	rows, err := s.db.Query(`
SELECT id, agent_key, text, created_at
FROM agent_notes
WHERE workspace_id = ? AND agent_key = ? AND text LIKE ?
ORDER BY id DESC
LIMIT ?
`, workspaceID, agentKey, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentNote
	for rows.Next() {
		var n AgentNote
		if err := rows.Scan(&n.ID, &n.AgentKey, &n.Text, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

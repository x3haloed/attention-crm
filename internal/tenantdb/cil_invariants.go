package tenantdb

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

type CILInvariant struct {
	ID       int64
	AgentKey string

	InvID   string
	Name    string
	Trigger string
	Because string
	IfNot   string
	Scope   string
	Seen    sql.NullInt64
	Last    sql.NullString
	Ex      sql.NullString
	Rev     string
	Src     string

	CreatedAt string
}

type CILInvariantInput struct {
	InvID   string
	Name    string
	Trigger string
	Because string
	IfNot   string
	Scope   string
	Seen    *int64
	Last    *string
	Ex      *string
	Rev     string
	Src     string
}

func (s *Store) AppendCILInvariant(agentKey string, in CILInvariantInput, sourceEventID *int64) (int64, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return 0, errors.New("agent key required")
	}
	in.InvID = strings.TrimSpace(in.InvID)
	in.Name = strings.TrimSpace(in.Name)
	in.Trigger = strings.TrimSpace(in.Trigger)
	in.Because = strings.TrimSpace(in.Because)
	in.IfNot = strings.TrimSpace(in.IfNot)
	in.Scope = strings.TrimSpace(in.Scope)
	in.Rev = strings.TrimSpace(in.Rev)
	in.Src = strings.TrimSpace(in.Src)
	if in.InvID == "" || in.Name == "" || in.Trigger == "" || in.Because == "" || in.IfNot == "" || in.Scope == "" || in.Rev == "" || in.Src == "" {
		return 0, errors.New("missing required invariant fields")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}

	var seen sql.NullInt64
	if in.Seen != nil && *in.Seen >= 0 {
		seen = sql.NullInt64{Int64: *in.Seen, Valid: true}
	}
	var last sql.NullString
	if in.Last != nil && strings.TrimSpace(*in.Last) != "" {
		last = sql.NullString{String: strings.TrimSpace(*in.Last), Valid: true}
	}
	var ex sql.NullString
	if in.Ex != nil && strings.TrimSpace(*in.Ex) != "" {
		ex = sql.NullString{String: strings.TrimSpace(*in.Ex), Valid: true}
	}
	var srcEv sql.NullInt64
	if sourceEventID != nil && *sourceEventID > 0 {
		srcEv = sql.NullInt64{Int64: *sourceEventID, Valid: true}
	}

	res, err := s.db.Exec(`
INSERT INTO cil_invariants(
  workspace_id, agent_key,
  inv_id, name, trigger, because, if_not, scope,
  seen, last, ex,
  rev, src,
  source_event_id, created_at
)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
`, workspaceID, agentKey,
		in.InvID, in.Name, in.Trigger, in.Because, in.IfNot, in.Scope,
		seen, last, ex,
		in.Rev, in.Src,
		srcEv, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (s *Store) CompileCILHead(agentKey string, maxLines int) (string, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return "", errors.New("agent key required")
	}
	if maxLines <= 0 || maxLines > 200 {
		maxLines = 30
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}

	rows, err := s.db.Query(`
WITH latest AS (
  SELECT
    inv_id, name, trigger, because, if_not, scope, seen, last, ex, rev, src,
    row_number() OVER (PARTITION BY inv_id ORDER BY id DESC) AS rn
  FROM cil_invariants
  WHERE workspace_id = ? AND agent_key = ?
)
SELECT inv_id, name, trigger, because, if_not, scope, seen, last, ex, rev, src
FROM latest
WHERE rn = 1 AND (rev = '' OR lower(rev) LIKE 'active%')
ORDER BY inv_id ASC
LIMIT ?
`, workspaceID, agentKey, maxLines)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var lines []string
	for rows.Next() {
		var invID, name, trigger, because, ifNot, scope, rev, src string
		var seen sql.NullInt64
		var last, ex sql.NullString
		if err := rows.Scan(&invID, &name, &trigger, &because, &ifNot, &scope, &seen, &last, &ex, &rev, &src); err != nil {
			return "", err
		}
		lines = append(lines, formatINVLine(CILInvariantInput{
			InvID:   invID,
			Name:    name,
			Trigger: trigger,
			Because: because,
			IfNot:   ifNot,
			Scope:   scope,
			Seen:    nullIntToPtr(seen),
			Last:    nullStringToPtr(last),
			Ex:      nullStringToPtr(ex),
			Rev:     rev,
			Src:     src,
		}))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

func nullStringToPtr(s sql.NullString) *string {
	if !s.Valid {
		return nil
	}
	v := s.String
	return &v
}

func nullIntToPtr(i sql.NullInt64) *int64 {
	if !i.Valid {
		return nil
	}
	v := i.Int64
	return &v
}

func invEscape(v string) string {
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	v = strings.ReplaceAll(v, "\t", " ")
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, ";", `\;`)
	return v
}

func formatINVLine(in CILInvariantInput) string {
	var b strings.Builder
	b.WriteString("INV: ")
	b.WriteString("id=" + invEscape(in.InvID) + "; ")
	b.WriteString("name=" + invEscape(in.Name) + "; ")
	b.WriteString("trigger=" + invEscape(in.Trigger) + "; ")
	b.WriteString("because=" + invEscape(in.Because) + "; ")
	b.WriteString("if_not=" + invEscape(in.IfNot) + "; ")
	b.WriteString("scope=" + invEscape(in.Scope) + "; ")
	if in.Seen != nil {
		b.WriteString("seen=" + invEscape(strconv.FormatInt(*in.Seen, 10)) + "; ")
	}
	if in.Last != nil {
		b.WriteString("last=" + invEscape(*in.Last) + "; ")
	}
	if in.Ex != nil {
		b.WriteString("ex=" + invEscape(*in.Ex) + "; ")
	}
	b.WriteString("rev=" + invEscape(in.Rev) + "; ")
	b.WriteString("src=" + invEscape(in.Src))
	return b.String()
}

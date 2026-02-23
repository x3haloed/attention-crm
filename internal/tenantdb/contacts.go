package tenantdb

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
)

func (s *Store) CreateContact(name, email, phone, company, notes string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("contact name required")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO contacts(workspace_id, name, email, phone, company, notes)
VALUES(?,?,?,?,?,?)
`, workspaceID, name, strings.TrimSpace(email), strings.TrimSpace(phone), strings.TrimSpace(company), strings.TrimSpace(notes))
	return err
}

func (s *Store) UpdateContactField(contactID int64, field, value string) (string, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}
	field = strings.TrimSpace(strings.ToLower(field))
	switch field {
	case "name":
		value = strings.TrimSpace(value)
		if value == "" {
			return "", errors.New("contact name required")
		}
	case "email":
		value = strings.ToLower(strings.TrimSpace(value))
	case "phone", "company":
		value = strings.TrimSpace(value)
	default:
		return "", errors.New("invalid field")
	}

	res, err := s.db.Exec(`
UPDATE contacts
SET `+field+` = ?, updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND workspace_id = ?
`, value, contactID, workspaceID)
	if err != nil {
		return "", err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return "", errors.New("contact not found")
	}

	var updatedAt string
	if err := s.db.QueryRow(`
SELECT updated_at
FROM contacts
WHERE id = ? AND workspace_id = ?
`, contactID, workspaceID).Scan(&updatedAt); err != nil {
		return "", err
	}
	return updatedAt, nil
}

func (s *Store) DuplicateCandidates(name, email, phone, company string, limit int) ([]DuplicateCandidate, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	email = strings.ToLower(strings.TrimSpace(email))
	phone = normalizePhone(strings.TrimSpace(phone))
	name = strings.TrimSpace(name)
	company = strings.TrimSpace(company)

	candidates := make(map[int64]DuplicateCandidate)
	add := func(c Contact, reason string) {
		if existing, ok := candidates[c.ID]; ok {
			existing.Reason = existing.Reason + ", " + reason
			candidates[c.ID] = existing
			return
		}
		candidates[c.ID] = DuplicateCandidate{Contact: c, Reason: reason}
	}

	if email != "" {
		rows, err := s.db.Query(`
SELECT id, name, email, phone, company, notes, created_at, updated_at
FROM contacts
WHERE workspace_id = ? AND lower(email) = ?
LIMIT ?
`, workspaceID, email, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c Contact
			if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			add(c, "email match")
		}
		rows.Close()
	}

	if phone != "" {
		rows, err := s.db.Query(`
SELECT id, name, email, phone, company, notes, created_at, updated_at
FROM contacts
WHERE workspace_id = ? AND replace(replace(replace(replace(phone,'-',''),' ',''),'(',''),')','') = ?
LIMIT ?
`, workspaceID, phone, limit)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c Contact
			if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
				rows.Close()
				return nil, err
			}
			add(c, "phone match")
		}
		rows.Close()
	}

	// Fuzzy match: use FTS query against name/company if available, else LIKE.
	queryParts := strings.Fields(strings.TrimSpace(name + " " + company))
	if len(queryParts) > 0 {
		q := strings.Join(queryParts, " ")
		contacts, _ := s.SearchContacts(q, limit)
		for _, c := range contacts {
			add(c, "name/company fuzzy")
		}
	}

	out := make([]DuplicateCandidate, 0, len(candidates))
	for _, dc := range candidates {
		out = append(out, dc)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Contact.UpdatedAt > out[j].Contact.UpdatedAt
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) ListContacts() ([]Contact, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT id, name, email, phone, company, notes, created_at, updated_at
FROM contacts
WHERE workspace_id = ?
ORDER BY updated_at DESC, id DESC
LIMIT 100
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (s *Store) ContactByID(contactID int64) (Contact, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return Contact{}, err
	}
	row := s.db.QueryRow(`
SELECT id, name, email, phone, company, notes, created_at, updated_at
FROM contacts
WHERE id = ? AND workspace_id = ?
`, contactID, workspaceID)
	var c Contact
	if err := row.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Contact{}, errors.New("contact not found")
		}
		return Contact{}, err
	}
	return c, nil
}

func normalizePhone(in string) string {
	var b strings.Builder
	for _, r := range in {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) ContactOptions() ([]Contact, error) {
	contacts, err := s.ListContacts()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(contacts, func(i, j int) bool {
		return strings.ToLower(contacts[i].Name) < strings.ToLower(contacts[j].Name)
	})
	return contacts, nil
}

func (s *Store) SearchContacts(query string, limit int) ([]Contact, error) {
	// Prefer FTS when available for the MVP "fuzzy search" experience.
	if contacts, ok, err := s.searchContactsFTS(query, limit); err != nil {
		return nil, err
	} else if ok {
		return contacts, nil
	}

	if limit <= 0 || limit > 100 {
		limit = 10
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	needle := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.Query(`
SELECT id, name, email, phone, company, notes, created_at, updated_at
FROM contacts
WHERE workspace_id = ?
  AND (
    lower(name) LIKE ?
    OR lower(email) LIKE ?
    OR lower(phone) LIKE ?
    OR lower(company) LIKE ?
  )
ORDER BY
  CASE WHEN lower(name) = ? THEN 0 ELSE 1 END,
  updated_at DESC,
  id DESC
LIMIT ?
`, workspaceID, needle, needle, needle, needle, strings.ToLower(strings.TrimSpace(query)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

func (s *Store) searchContactsFTS(query string, limit int) ([]Contact, bool, error) {
	query = strings.TrimSpace(query)
	if len(query) < 2 {
		return nil, false, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if ok, err := s.hasTable("contacts_fts"); err != nil || !ok {
		return nil, ok, err
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, true, err
	}

	exactLower := strings.ToLower(query)
	prefixLower := strings.ToLower(query) + "%"
	ftsQuery := buildFTSQuery(query)
	rows, err := s.db.Query(`
SELECT c.id, c.name, c.email, c.phone, c.company, c.notes, c.created_at, c.updated_at
FROM contacts_fts f
JOIN contacts c ON c.id = f.rowid
WHERE c.workspace_id = ?
  AND contacts_fts MATCH ?
ORDER BY
  CASE WHEN lower(c.name) = ? THEN 0 ELSE 1 END,
  CASE WHEN lower(c.name) LIKE ? THEN 0 ELSE 1 END,
  bm25(contacts_fts, 6.0, 4.0, 4.0, 2.0, 1.0),
  c.updated_at DESC,
  c.id DESC
LIMIT ?
`, workspaceID, ftsQuery, exactLower, prefixLower, limit)
	if err != nil {
		// If FTS5 isn't compiled/available, fall back to LIKE.
		return nil, false, nil
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var c Contact
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, true, err
		}
		contacts = append(contacts, c)
	}
	return contacts, true, rows.Err()
}

func buildFTSQuery(input string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(input)))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, "\"'")
		p = escapeFTSToken(p)
		if p == "" {
			continue
		}
		// Prefix match per token for fast "typeahead" feel.
		out = append(out, p+"*")
	}
	if len(out) == 0 {
		return input
	}
	return strings.Join(out, " ")
}

func escapeFTSToken(token string) string {
	// Keep this conservative: allow alnum and a few safe chars, drop the rest.
	// This prevents accidental FTS query operators from user input.
	var b strings.Builder
	for _, r := range token {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '\'':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) hasTable(name string) (bool, error) {
	row := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type IN ('table','view') AND name = ?)`, name)
	var exists int
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists == 1, nil
}

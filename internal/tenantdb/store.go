package tenantdb

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID    int64
	Email string
	Name  string
}

type WebAuthnUser struct {
	User
	Credentials []webauthn.Credential
}

func (u WebAuthnUser) WebAuthnID() []byte {
	return []byte(fmt.Sprintf("user:%d", u.ID))
}

func (u WebAuthnUser) WebAuthnName() string {
	return u.Email
}

func (u WebAuthnUser) WebAuthnDisplayName() string {
	return u.Name
}

func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

type Contact struct {
	ID        int64
	Name      string
	Email     string
	Phone     string
	Company   string
	Notes     string
	CreatedAt string
	UpdatedAt string
}

type DuplicateCandidate struct {
	Contact Contact
	Reason  string
}

type Interaction struct {
	ID          int64
	ContactID   int64
	ContactName string
	Type        string
	Content     string
	DueAt       sql.NullString
	CompletedAt sql.NullString
	CreatedAt   string
}

type Deal struct {
	ID                int64
	Title             string
	State             string
	ValueCents        sql.NullInt64
	StageLabel        string
	NextStep          string
	NextStepDueAt     sql.NullString
	NextStepCompleted sql.NullString
	CloseWindowStart  sql.NullString
	CloseWindowEnd    sql.NullString
	ClosedAt          sql.NullString
	ClosedOutcome     string
	LastActivityAt    string
	CreatedAt         string
	UpdatedAt         string
}

type DealPipelineRow struct {
	Deal
	PrimaryContactID      int64
	PrimaryContactName    string
	PrimaryContactCompany string
	ContactCount          int
}

type DealEvent struct {
	ID        int64
	DealID    int64
	Type      string
	Content   string
	CreatedAt string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir tenant db parent: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open tenant db: %w", err)
	}
	if err := applyPragmas(db); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) HasUsers() (bool, error) {
	row := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users LIMIT 1)`)
	var exists int
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists == 1, nil
}

func (s *Store) CreateInitialUser(workspaceName, email, name, password string) error {
	hasUsers, err := s.HasUsers()
	if err != nil {
		return err
	}
	if hasUsers {
		return errors.New("users already exist")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO workspaces(name) VALUES(?)`, workspaceName)
	if err != nil {
		return err
	}
	workspaceID, err := res.LastInsertId()
	if err != nil {
		return err
	}

	userRes, err := tx.Exec(`INSERT INTO users(email, name, password_hash) VALUES(?,?,?)`, email, name, string(hash))
	if err != nil {
		return err
	}
	userID, err := userRes.LastInsertId()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO memberships(workspace_id, user_id, is_owner) VALUES(?,?,1)`, workspaceID, userID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) CreateInitialUserForPasskey(workspaceName, email, name string) (User, error) {
	hasUsers, err := s.HasUsers()
	if err != nil {
		return User{}, err
	}
	if hasUsers {
		return User{}, errors.New("users already exist")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" || email == "" || name == "" {
		return User{}, errors.New("workspace, email, and name are required")
	}

	passwordSeed := make([]byte, 32)
	if _, err := rand.Read(passwordSeed); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword(passwordSeed, bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`INSERT INTO workspaces(name) VALUES(?)`, workspaceName)
	if err != nil {
		return User{}, err
	}
	workspaceID, err := res.LastInsertId()
	if err != nil {
		return User{}, err
	}

	userRes, err := tx.Exec(`INSERT INTO users(email, name, password_hash) VALUES(?,?,?)`, email, name, string(hash))
	if err != nil {
		return User{}, err
	}
	userID, err := userRes.LastInsertId()
	if err != nil {
		return User{}, err
	}

	if _, err := tx.Exec(`INSERT INTO memberships(workspace_id, user_id, is_owner) VALUES(?,?,1)`, workspaceID, userID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}

	return User{ID: userID, Email: email, Name: name}, nil
}

func (s *Store) Authenticate(email, password string) (User, error) {
	row := s.db.QueryRow(`SELECT id, email, name, password_hash FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(email)))
	var user User
	var passwordHash string
	if err := row.Scan(&user.ID, &user.Email, &user.Name, &passwordHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}

	_, _ = s.db.Exec(`UPDATE users SET last_login_at = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339Nano), user.ID)
	return user, nil
}

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

func normalizePhone(in string) string {
	var b strings.Builder
	for _, r := range in {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *Store) CreateInteraction(contactID int64, interactionType, content string, dueAt *time.Time) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	interactionType = strings.TrimSpace(strings.ToLower(interactionType))
	switch interactionType {
	case "note", "call", "email", "meeting":
	default:
		return errors.New("invalid interaction type")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("content required")
	}

	var dueAtStr any
	if dueAt != nil {
		dueAtStr = dueAt.UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
INSERT INTO interactions(workspace_id, contact_id, type, content, due_at)
VALUES(?,?,?,?,?)
`, workspaceID, contactID, interactionType, content, dueAtStr)
	if err != nil {
		return err
	}

	// Touch contact updated_at so recency-based UIs behave like a real CRM.
	if _, err := tx.Exec(`
UPDATE contacts
SET updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND workspace_id = ?
`, contactID, workspaceID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListRecentInteractions(limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT i.id, i.contact_id, c.name, i.type, i.content, i.due_at, i.completed_at, i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
WHERE i.workspace_id = ?
ORDER BY i.created_at DESC, i.id DESC
LIMIT ?
`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.ContactID, &it.ContactName, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) ListNeedsAttention(limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
SELECT i.id, i.contact_id, c.name, i.type, i.content, i.due_at, i.completed_at, i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
WHERE i.workspace_id = ?
  AND i.due_at IS NOT NULL
  AND i.completed_at IS NULL
ORDER BY i.due_at ASC, i.id ASC
LIMIT ?
`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.ContactID, &it.ContactName, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) ListDealsNeedsAttention(limit int) ([]Deal, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	cutoff := now.Add(48 * time.Hour).Format(time.RFC3339Nano)

	rows, err := s.db.Query(`
SELECT id, title, state, value_cents, stage_label, next_step, next_step_due_at, next_step_completed_at,
       close_window_start, close_window_end, closed_at, closed_outcome, last_activity_at, created_at, updated_at
FROM deals
WHERE workspace_id = ?
  AND state = 'open'
  AND (
    TRIM(COALESCE(next_step, '')) = ''
    OR (next_step_due_at IS NOT NULL AND next_step_due_at != '' AND next_step_completed_at IS NULL AND next_step_due_at <= ?)
  )
ORDER BY
  CASE WHEN TRIM(COALESCE(next_step, '')) = '' THEN 0 ELSE 1 END ASC,
  COALESCE(next_step_due_at, '') ASC,
  last_activity_at DESC,
  id DESC
LIMIT ?
`, workspaceID, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Deal
	for rows.Next() {
		var d Deal
		if err := rows.Scan(
			&d.ID,
			&d.Title,
			&d.State,
			&d.ValueCents,
			&d.StageLabel,
			&d.NextStep,
			&d.NextStepDueAt,
			&d.NextStepCompleted,
			&d.CloseWindowStart,
			&d.CloseWindowEnd,
			&d.ClosedAt,
			&d.ClosedOutcome,
			&d.LastActivityAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListDealsPipeline(limit int) ([]DealPipelineRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	cutoff := now.Add(48 * time.Hour).Format(time.RFC3339Nano)

	rows, err := s.db.Query(`
SELECT
  d.id, d.title, d.state, d.value_cents, d.stage_label, d.next_step, d.next_step_due_at, d.next_step_completed_at,
  d.close_window_start, d.close_window_end, d.closed_at, d.closed_outcome, d.last_activity_at, d.created_at, d.updated_at,
  MIN(c.id) AS primary_contact_id,
  MIN(c.name) AS primary_contact_name,
  MIN(c.company) AS primary_contact_company,
  COUNT(dc.contact_id) AS contact_count
FROM deals d
JOIN deal_contacts dc ON dc.deal_id = d.id
JOIN contacts c ON c.id = dc.contact_id
WHERE d.workspace_id = ?
  AND d.state = 'open'
GROUP BY d.id
ORDER BY
  CASE WHEN TRIM(COALESCE(d.next_step, '')) = '' THEN 0 ELSE 1 END ASC,
  CASE
    WHEN d.next_step_due_at IS NOT NULL AND d.next_step_due_at != '' AND d.next_step_completed_at IS NULL AND d.next_step_due_at <= ? THEN 0
    ELSE 1
  END ASC,
  COALESCE(d.next_step_due_at, '') ASC,
  d.last_activity_at DESC,
  d.id DESC
LIMIT ?
`, workspaceID, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DealPipelineRow
	for rows.Next() {
		var row DealPipelineRow
		if err := rows.Scan(
			&row.ID,
			&row.Title,
			&row.State,
			&row.ValueCents,
			&row.StageLabel,
			&row.NextStep,
			&row.NextStepDueAt,
			&row.NextStepCompleted,
			&row.CloseWindowStart,
			&row.CloseWindowEnd,
			&row.ClosedAt,
			&row.ClosedOutcome,
			&row.LastActivityAt,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.PrimaryContactID,
			&row.PrimaryContactName,
			&row.PrimaryContactCompany,
			&row.ContactCount,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) MarkInteractionComplete(interactionID int64) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var contactID int64
	row := tx.QueryRow(`SELECT contact_id FROM interactions WHERE id = ? AND workspace_id = ?`, interactionID, workspaceID)
	if err := row.Scan(&contactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("interaction not found or already completed")
		}
		return err
	}

	res, err := tx.Exec(`
UPDATE interactions
SET completed_at = ?
WHERE id = ? AND workspace_id = ? AND completed_at IS NULL
`, time.Now().UTC().Format(time.RFC3339), interactionID, workspaceID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("interaction not found or already completed")
	}

	if _, err := tx.Exec(`
UPDATE contacts
SET updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND workspace_id = ?
`, contactID, workspaceID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) CreateDeal(title string, contactIDs []int64) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, errors.New("deal title required")
	}
	if len(contactIDs) == 0 {
		return 0, errors.New("deal must attach to at least one contact")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO deals(workspace_id, title, state, last_activity_at, created_at, updated_at) VALUES(?, ?, 'open', ?, ?, ?)`,
		workspaceID, title, now, now, now,
	)
	if err != nil {
		return 0, err
	}
	dealID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	seen := map[int64]bool{}
	for _, cid := range contactIDs {
		if cid <= 0 || seen[cid] {
			continue
		}
		seen[cid] = true
		if _, err := tx.Exec(`INSERT INTO deal_contacts(deal_id, contact_id) VALUES(?, ?)`, dealID, cid); err != nil {
			return 0, err
		}
	}
	if len(seen) == 0 {
		return 0, errors.New("deal must attach to at least one contact")
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return dealID, nil
}

func (s *Store) DealByID(dealID int64) (Deal, []int64, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return Deal{}, nil, err
	}
	row := s.db.QueryRow(`
SELECT id, title, state, value_cents, stage_label, next_step, next_step_due_at, next_step_completed_at,
       close_window_start, close_window_end, closed_at, closed_outcome, last_activity_at, created_at, updated_at
FROM deals
WHERE workspace_id = ? AND id = ?
`, workspaceID, dealID)
	var d Deal
	if err := row.Scan(
		&d.ID,
		&d.Title,
		&d.State,
		&d.ValueCents,
		&d.StageLabel,
		&d.NextStep,
		&d.NextStepDueAt,
		&d.NextStepCompleted,
		&d.CloseWindowStart,
		&d.CloseWindowEnd,
		&d.ClosedAt,
		&d.ClosedOutcome,
		&d.LastActivityAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	); err != nil {
		return Deal{}, nil, err
	}

	rows, err := s.db.Query(`SELECT contact_id FROM deal_contacts WHERE deal_id = ? ORDER BY contact_id`, dealID)
	if err != nil {
		return Deal{}, nil, err
	}
	defer rows.Close()
	var contactIDs []int64
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			return Deal{}, nil, err
		}
		contactIDs = append(contactIDs, cid)
	}
	if err := rows.Err(); err != nil {
		return Deal{}, nil, err
	}
	return d, contactIDs, nil
}

func (s *Store) ListDealsByContact(contactID int64, limit int) ([]Deal, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT d.id, d.title, d.state, d.value_cents, d.stage_label, d.next_step, d.next_step_due_at, d.next_step_completed_at,
       d.close_window_start, d.close_window_end, d.closed_at, d.closed_outcome, d.last_activity_at, d.created_at, d.updated_at
FROM deals d
JOIN deal_contacts dc ON dc.deal_id = d.id
WHERE d.workspace_id = ? AND dc.contact_id = ?
ORDER BY d.last_activity_at DESC, d.id DESC
LIMIT ?
`, workspaceID, contactID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deal
	for rows.Next() {
		var d Deal
		if err := rows.Scan(
			&d.ID,
			&d.Title,
			&d.State,
			&d.ValueCents,
			&d.StageLabel,
			&d.NextStep,
			&d.NextStepDueAt,
			&d.NextStepCompleted,
			&d.CloseWindowStart,
			&d.CloseWindowEnd,
			&d.ClosedAt,
			&d.ClosedOutcome,
			&d.LastActivityAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateDealNextStep(dealID int64, nextStep string, dueAt *time.Time) (string, error) {
	nextStep = strings.TrimSpace(nextStep)
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var dueAtStr any
	if dueAt != nil {
		dueAtStr = dueAt.UTC().Format(time.RFC3339Nano)
	}

	res, err := s.db.Exec(`
UPDATE deals
SET next_step = ?, next_step_due_at = ?, next_step_completed_at = NULL, updated_at = ?
WHERE workspace_id = ? AND id = ? AND state = 'open'
`, nextStep, dueAtStr, now, workspaceID, dealID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return now, nil
}

func (s *Store) CompleteDealNextStep(dealID int64) (string, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`
UPDATE deals
SET next_step_completed_at = ?, updated_at = ?
WHERE workspace_id = ? AND id = ? AND state = 'open' AND next_step != '' AND next_step_completed_at IS NULL
`, now, now, workspaceID, dealID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return now, nil
}

func (s *Store) CloseDeal(dealID int64, state, outcome string) (string, error) {
	state = strings.TrimSpace(strings.ToLower(state))
	if state != "won" && state != "lost" {
		return "", errors.New("close state must be won or lost")
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		return "", errors.New("close outcome required")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`
UPDATE deals
SET state = ?, closed_at = ?, closed_outcome = ?, updated_at = ?
WHERE workspace_id = ? AND id = ? AND state = 'open'
`, state, now, outcome, now, workspaceID, dealID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return now, nil
}

func (s *Store) CreateDealEvent(dealID int64, eventType, content string) error {
	eventType = strings.TrimSpace(strings.ToLower(eventType))
	switch eventType {
	case "note", "call", "email", "meeting", "system":
	default:
		return errors.New("invalid deal event type")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("deal event content required")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`INSERT INTO deal_events(workspace_id, deal_id, type, content, created_at) VALUES(?, ?, ?, ?, ?)`,
		workspaceID, dealID, eventType, content, now,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE deals SET last_activity_at = ?, updated_at = ? WHERE workspace_id = ? AND id = ?`,
		now, now, workspaceID, dealID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListDealEvents(dealID int64, limit int) ([]DealEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT id, deal_id, type, content, created_at
FROM deal_events
WHERE workspace_id = ? AND deal_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, workspaceID, dealID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DealEvent
	for rows.Next() {
		var ev DealEvent
		if err := rows.Scan(&ev.ID, &ev.DealID, &ev.Type, &ev.Content, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type Invite struct {
	ID          int64
	Email       string
	CreatedByID int64
	CreatedAt   string
	ExpiresAt   string
	RedeemedAt  sql.NullString
	StartedAt   sql.NullString
	StartedUser sql.NullInt64
	RevokedAt   sql.NullString
}

type Member struct {
	UserID    int64
	Email     string
	Name      string
	IsOwner   bool
	CreatedAt string
}

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

func (s *Store) CreateInvite(createdByUserID int64, email string, ttl time.Duration) (token string, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return "", errors.New("email required")
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}
	isOwner, err := s.IsOwner(createdByUserID)
	if err != nil {
		return "", err
	}
	if !isOwner {
		return "", errors.New("only owner can create invites")
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))

	expiresAt := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	_, err = s.db.Exec(`
INSERT INTO invites(workspace_id, token_hash, email, created_by_user_id, expires_at)
VALUES(?,?,?,?,?)
`, workspaceID, hash[:], email, createdByUserID, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) InviteByToken(token string) (Invite, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return Invite{}, err
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(token)))
	row := s.db.QueryRow(`
SELECT id, email, created_by_user_id, created_at, expires_at, redeemed_at, redeem_started_at, redeem_user_id, revoked_at
FROM invites
WHERE workspace_id = ? AND token_hash = ?
`, workspaceID, hash[:])
	var inv Invite
	if err := row.Scan(&inv.ID, &inv.Email, &inv.CreatedByID, &inv.CreatedAt, &inv.ExpiresAt, &inv.RedeemedAt, &inv.StartedAt, &inv.StartedUser, &inv.RevokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Invite{}, errors.New("invite not found")
		}
		return Invite{}, err
	}
	if inv.RevokedAt.Valid {
		return Invite{}, errors.New("invite revoked")
	}
	return inv, nil
}

func (s *Store) StartInviteRedemption(token, name string) (User, error) {
	token = strings.TrimSpace(token)
	name = strings.TrimSpace(name)
	if token == "" || name == "" {
		return User{}, errors.New("token and name required")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return User{}, err
	}
	hash := sha256.Sum256([]byte(token))

	tx, err := s.db.Begin()
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
SELECT id, email, expires_at, redeemed_at, redeem_user_id, revoked_at
FROM invites
WHERE workspace_id = ? AND token_hash = ?
`, workspaceID, hash[:])
	var inviteID int64
	var email string
	var expiresAt string
	var redeemedAt sql.NullString
	var redeemUserID sql.NullInt64
	var revokedAt sql.NullString
	if err := row.Scan(&inviteID, &email, &expiresAt, &redeemedAt, &redeemUserID, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, errors.New("invite not found")
		}
		return User{}, err
	}
	if revokedAt.Valid {
		return User{}, errors.New("invite revoked")
	}
	if redeemedAt.Valid {
		return User{}, errors.New("invite already redeemed")
	}
	expTime, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return User{}, errors.New("invite expiry invalid")
	}
	if time.Now().UTC().After(expTime) {
		return User{}, errors.New("invite expired")
	}

	if redeemUserID.Valid {
		userRow := tx.QueryRow(`SELECT id, email, name FROM users WHERE id = ?`, redeemUserID.Int64)
		var u User
		if err := userRow.Scan(&u.ID, &u.Email, &u.Name); err != nil {
			return User{}, err
		}
		if err := tx.Commit(); err != nil {
			return User{}, err
		}
		return u, nil
	}

	passwordSeed := make([]byte, 32)
	if _, err := rand.Read(passwordSeed); err != nil {
		return User{}, err
	}
	hashBytes, err := bcrypt.GenerateFromPassword(passwordSeed, bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	userRes, err := tx.Exec(`INSERT INTO users(email, name, password_hash) VALUES(?,?,?)`, email, name, string(hashBytes))
	if err != nil {
		return User{}, err
	}
	userID, err := userRes.LastInsertId()
	if err != nil {
		return User{}, err
	}

	if _, err := tx.Exec(`INSERT INTO memberships(workspace_id, user_id, is_owner) VALUES(?,?,0)`, workspaceID, userID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(`UPDATE invites SET redeem_started_at = ?, redeem_user_id = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), userID, inviteID); err != nil {
		return User{}, err
	}

	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return User{ID: userID, Email: email, Name: name}, nil
}

func (s *Store) CompleteInviteRedemption(token string, userID int64) error {
	token = strings.TrimSpace(token)
	if token == "" || userID <= 0 {
		return errors.New("token and user required")
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(token))
	res, err := s.db.Exec(`
UPDATE invites
SET redeemed_at = ?
WHERE workspace_id = ?
  AND token_hash = ?
  AND redeem_user_id = ?
  AND revoked_at IS NULL
  AND redeemed_at IS NULL
`, time.Now().UTC().Format(time.RFC3339), workspaceID, hash[:], userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("invite not found or already redeemed")
	}
	return nil
}

func (s *Store) ListInvites(limit int) ([]Invite, error) {
	if limit <= 0 {
		limit = 100
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT id, email, created_by_user_id, created_at, expires_at, redeemed_at, redeem_started_at, redeem_user_id, revoked_at
FROM invites
WHERE workspace_id = ?
ORDER BY created_at DESC
LIMIT ?
`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Invite
	for rows.Next() {
		var inv Invite
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.CreatedByID, &inv.CreatedAt, &inv.ExpiresAt, &inv.RedeemedAt, &inv.StartedAt, &inv.StartedUser, &inv.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *Store) RevokeInvite(inviteID int64, byUserID int64) error {
	if inviteID <= 0 || byUserID <= 0 {
		return errors.New("invite id and user required")
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	isOwner, err := s.IsOwner(byUserID)
	if err != nil {
		return err
	}
	if !isOwner {
		return errors.New("only owner can revoke invites")
	}
	res, err := s.db.Exec(`
UPDATE invites
SET revoked_at = ?
WHERE workspace_id = ?
  AND id = ?
  AND redeemed_at IS NULL
  AND revoked_at IS NULL
`, time.Now().UTC().Format(time.RFC3339), workspaceID, inviteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("invite not found or not revocable")
	}
	return nil
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

func (s *Store) WebAuthnUserByEmail(email string) (WebAuthnUser, error) {
	row := s.db.QueryRow(`SELECT id, email, name FROM users WHERE email = ?`, strings.ToLower(strings.TrimSpace(email)))
	var user WebAuthnUser
	if err := row.Scan(&user.ID, &user.Email, &user.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebAuthnUser{}, errors.New("user not found")
		}
		return WebAuthnUser{}, err
	}
	creds, err := s.credentialsByUserID(user.ID)
	if err != nil {
		return WebAuthnUser{}, err
	}
	user.Credentials = creds
	return user, nil
}

func (s *Store) WebAuthnUserByID(userID int64) (WebAuthnUser, error) {
	row := s.db.QueryRow(`SELECT id, email, name FROM users WHERE id = ?`, userID)
	var user WebAuthnUser
	if err := row.Scan(&user.ID, &user.Email, &user.Name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WebAuthnUser{}, errors.New("user not found")
		}
		return WebAuthnUser{}, err
	}
	creds, err := s.credentialsByUserID(user.ID)
	if err != nil {
		return WebAuthnUser{}, err
	}
	user.Credentials = creds
	return user, nil
}

func (s *Store) AddWebAuthnCredential(userID int64, credential *webauthn.Credential) error {
	if credential == nil {
		return errors.New("credential required")
	}
	payload, err := json.Marshal(credential)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
INSERT INTO passkey_credentials(user_id, credential_id, credential_json)
VALUES(?,?,?)
ON CONFLICT(credential_id) DO UPDATE SET
  user_id = excluded.user_id,
  credential_json = excluded.credential_json
`, userID, credential.ID, string(payload))
	return err
}

func (s *Store) credentialsByUserID(userID int64) ([]webauthn.Credential, error) {
	rows, err := s.db.Query(`
SELECT credential_json
FROM passkey_credentials
WHERE user_id = ?
ORDER BY id DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []webauthn.Credential
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(payload), &cred); err != nil {
			return nil, err
		}
		out = append(out, cred)
	}
	return out, rows.Err()
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

func (s *Store) ListInteractionsByContact(contactID int64, limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT i.id, i.contact_id, c.name, i.type, i.content, i.due_at, i.completed_at, i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
WHERE i.workspace_id = ? AND i.contact_id = ?
ORDER BY i.created_at DESC, i.id DESC
LIMIT ?
`, workspaceID, contactID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.ContactID, &it.ContactName, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

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

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS workspaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  last_login_at TEXT
);

CREATE TABLE IF NOT EXISTS memberships (
  workspace_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  is_owner INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY(workspace_id, user_id),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS invites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  token_hash BLOB NOT NULL UNIQUE,
  email TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  expires_at TEXT NOT NULL,
  redeemed_at TEXT,
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(created_by_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_invites_workspace
  ON invites(workspace_id, created_at DESC);

CREATE TABLE IF NOT EXISTS contacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  company TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id)
);

CREATE INDEX IF NOT EXISTS idx_contacts_workspace_name
  ON contacts(workspace_id, name);

CREATE VIRTUAL TABLE IF NOT EXISTS contacts_fts USING fts5(
  name,
  email,
  phone,
  company,
  notes,
  content='contacts',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS contacts_ai AFTER INSERT ON contacts BEGIN
  INSERT INTO contacts_fts(rowid, name, email, phone, company, notes)
  VALUES (new.id, new.name, new.email, new.phone, new.company, new.notes);
END;

CREATE TRIGGER IF NOT EXISTS contacts_ad AFTER DELETE ON contacts BEGIN
  INSERT INTO contacts_fts(contacts_fts, rowid, name, email, phone, company, notes)
  VALUES ('delete', old.id, old.name, old.email, old.phone, old.company, old.notes);
END;

CREATE TRIGGER IF NOT EXISTS contacts_au AFTER UPDATE ON contacts BEGIN
  INSERT INTO contacts_fts(contacts_fts, rowid, name, email, phone, company, notes)
  VALUES ('delete', old.id, old.name, old.email, old.phone, old.company, old.notes);
  INSERT INTO contacts_fts(rowid, name, email, phone, company, notes)
  VALUES (new.id, new.name, new.email, new.phone, new.company, new.notes);
END;

CREATE TABLE IF NOT EXISTS interactions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  contact_id INTEGER NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('note','call','email','meeting')),
  content TEXT NOT NULL,
  due_at TEXT,
  completed_at TEXT,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(contact_id) REFERENCES contacts(id)
);

CREATE INDEX IF NOT EXISTS idx_interactions_workspace_created
  ON interactions(workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_interactions_workspace_due
  ON interactions(workspace_id, due_at, completed_at);

CREATE TABLE IF NOT EXISTS deals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'open' CHECK(state IN ('open','won','lost')),
  value_cents INTEGER,
  stage_label TEXT NOT NULL DEFAULT '',
  next_step TEXT NOT NULL DEFAULT '',
  next_step_due_at TEXT,
  next_step_completed_at TEXT,
  close_window_start TEXT,
  close_window_end TEXT,
  closed_at TEXT,
  closed_outcome TEXT NOT NULL DEFAULT '',
  last_activity_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id)
);

CREATE INDEX IF NOT EXISTS idx_deals_workspace_state
  ON deals(workspace_id, state, last_activity_at DESC);

CREATE INDEX IF NOT EXISTS idx_deals_workspace_next_step_due
  ON deals(workspace_id, state, next_step_due_at, next_step_completed_at);

CREATE TABLE IF NOT EXISTS deal_contacts (
  deal_id INTEGER NOT NULL,
  contact_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY(deal_id, contact_id),
  FOREIGN KEY(deal_id) REFERENCES deals(id) ON DELETE CASCADE,
  FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_deal_contacts_contact
  ON deal_contacts(contact_id, deal_id);

CREATE TABLE IF NOT EXISTS deal_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  deal_id INTEGER NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('note','call','email','meeting','system')),
  content TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(deal_id) REFERENCES deals(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_deal_events_deal_created
  ON deal_events(workspace_id, deal_id, created_at DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS interactions_fts USING fts5(
  content,
  content='interactions',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS interactions_ai AFTER INSERT ON interactions BEGIN
  INSERT INTO interactions_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS interactions_ad AFTER DELETE ON interactions BEGIN
  INSERT INTO interactions_fts(interactions_fts, rowid, content) VALUES ('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS interactions_au AFTER UPDATE ON interactions BEGIN
  INSERT INTO interactions_fts(interactions_fts, rowid, content) VALUES ('delete', old.id, old.content);
  INSERT INTO interactions_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TABLE IF NOT EXISTS passkey_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  credential_id BLOB NOT NULL UNIQUE,
  credential_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_passkey_credentials_user
  ON passkey_credentials(user_id);
`)
	if err != nil {
		return fmt.Errorf("migrate tenant: %w", err)
	}
	// Add newer invite fields for safe multi-step redemption.
	_ = execAddColumn(db, "invites", "redeem_started_at TEXT")
	_ = execAddColumn(db, "invites", "redeem_user_id INTEGER")
	_ = execAddColumn(db, "invites", "revoked_at TEXT")
	return nil
}

func execAddColumn(db *sql.DB, table, columnDef string) error {
	_, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + columnDef)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
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

var ErrInvalidCredentials = errors.New("invalid credentials")

package tenantdb

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

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

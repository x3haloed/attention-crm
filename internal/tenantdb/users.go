package tenantdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")

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

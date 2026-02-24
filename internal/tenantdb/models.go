package tenantdb

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/go-webauthn/webauthn/webauthn"
)

type Store struct {
	db       *sql.DB
	lockFile *os.File
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
	CreatedByID sql.NullInt64
	CreatedBy   sql.NullString
	UpdatedByID sql.NullInt64
	UpdatedBy   sql.NullString
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
	ID          int64
	DealID      int64
	Type        string
	Content     string
	CreatedByID sql.NullInt64
	CreatedBy   sql.NullString
	UpdatedByID sql.NullInt64
	UpdatedBy   sql.NullString
	CreatedAt   string
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

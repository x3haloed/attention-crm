package app

import (
	"errors"
	"html/template"
	"sync"
	"time"

	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/time/rate"
)

type Server struct {
	cfg          Config
	control      *control.Store
	sessionKey   []byte
	rootTmpl     *template.Template
	tenantAuth   *template.Template
	tenantApp    *template.Template
	webauthn     *webauthn.WebAuthn
	flowMu       sync.Mutex
	webauthnFlow map[string]ceremonyFlow

	limMu    sync.Mutex
	lim      map[string]*rate.Limiter
	limSeen  map[string]time.Time
	limSweep time.Time
}

type ceremonyFlow struct {
	TenantSlug string
	UserID     int64
	Session    webauthn.SessionData
	ExpiresAt  time.Time
}

func NewServer(cfg Config) (*Server, error) {
	controlStore, err := control.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	key, err := loadOrCreateSessionKey(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	rootTmpl := template.Must(template.New("root").Parse(rootPageTemplate))
	tenantBase := template.Must(template.New("tenant_base").Parse(tenantBaseTemplate))
	tenantAuth := template.Must(template.Must(tenantBase.Clone()).Parse(tenantAuthTemplate))
	tenantApp := template.Must(template.Must(tenantBase.Clone()).Parse(tenantAppTemplate))
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.WebAuthnRPID,
		RPDisplayName: cfg.WebAuthnName,
		RPOrigins:     cfg.WebAuthnOrigins,
	})
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:          cfg,
		control:      controlStore,
		sessionKey:   key,
		rootTmpl:     rootTmpl,
		tenantAuth:   tenantAuth,
		tenantApp:    tenantApp,
		webauthn:     wa,
		webauthnFlow: map[string]ceremonyFlow{},
		lim:          map[string]*rate.Limiter{},
		limSeen:      map[string]time.Time{},
	}

	if cfg.DevNoAuth {
		_ = s.ensureDevFixture()
	}

	return s, nil
}

func (s *Server) ensureDevFixture() error {
	const slug = "acme"
	const workspace = "Acme"

	tenant, err := s.control.TenantBySlug(slug)
	if err != nil {
		if errors.Is(err, control.ErrTenantNotFound) {
			tenant, err = s.control.CreateTenant(slug, workspace)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	hasUsers, err := db.HasUsers()
	if err != nil {
		return err
	}
	if !hasUsers {
		if _, err := db.CreateInitialUserForPasskey(workspace, "owner@example.com", "Owner"); err != nil {
			return err
		}
	}

	existing, _ := db.SearchContacts("Sarah", 1)
	if len(existing) == 0 {
		_ = db.CreateContact("Sarah Chen", "sarah.chen@acmecorp.com", "+1 (555) 123-4567", "Acme Corporation", "")
		_ = db.CreateContact("Bob Smith", "bob@betacorp.com", "", "Beta Corp", "")
		_ = db.CreateContact("Alex Johnson", "alex@startup.io", "", "Startup.io", "")
	}

	sarah, _ := db.SearchContacts("Sarah Chen", 1)
	if len(sarah) == 1 {
		contactID := sarah[0].ID
		recent, _ := db.ListRecentInteractions(1)
		if len(recent) == 0 {
			due := time.Now().Add(2 * time.Hour)
			_ = db.CreateInteraction(contactID, "call", "Follow up call scheduled", &due)
			_ = db.CreateInteraction(contactID, "note", "Called about Q1 budget planning", nil)
		}
	}

	return nil
}

type omniContact struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Company   string `json:"company"`
	UpdatedAt string `json:"updated_at"`
}

type omniAction struct {
	Type            string `json:"type"`
	ContactID       int64  `json:"contact_id,omitempty"`
	ContactName     string `json:"contact_name,omitempty"`
	InteractionType string `json:"interaction_type,omitempty"`
	Content         string `json:"content,omitempty"`
	DueAt           string `json:"due_at,omitempty"`
	Name            string `json:"name,omitempty"`
}

type session struct {
	TenantSlug string
	UserID     int64
}

type pageData struct {
	Title     string
	Header    template.HTML
	OmniBar   template.HTML
	MainID    string
	MainClass string
	Body      template.HTML
	CSRFToken string
}

type appViewState struct {
	Flash         string
	SearchResults []tenantdb.Contact
	UniversalText string
	InviteLink    string
	SuggestNote   *noteSuggestion
	Duplicates    []tenantdb.DuplicateCandidate
}

type noteSuggestion struct {
	Content      string
	DueAtLocal   string
	ContactHints []tenantdb.Contact
}

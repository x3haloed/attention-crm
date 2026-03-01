package control

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type InferenceConfig struct {
	TenantSlug string
	Provider   string
	BaseURL    string
	Model      string
	APIKey     string
	HeadersJSON string
	UpdatedAt  string
}

func normalizeProvider(p string) string {
	p = strings.TrimSpace(strings.ToLower(p))
	switch p {
	case "openai", "openrouter", "lmstudio":
		return p
	default:
		return ""
	}
}

func (s *Store) TenantInferenceConfig(tenantSlug string) (*InferenceConfig, error) {
	tenantSlug = sanitizeSlug(tenantSlug)
	if tenantSlug == "" {
		return nil, errors.New("tenant slug required")
	}
	row := s.db.QueryRow(`
SELECT tenant_slug, provider, base_url, model, api_key, headers_json, updated_at
FROM tenant_inference_config
WHERE tenant_slug = ?
`, tenantSlug)
	var cfg InferenceConfig
	if err := row.Scan(&cfg.TenantSlug, &cfg.Provider, &cfg.BaseURL, &cfg.Model, &cfg.APIKey, &cfg.HeadersJSON, &cfg.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &cfg, nil
}

func (s *Store) UpsertTenantInferenceConfig(cfg InferenceConfig) error {
	tenantSlug := sanitizeSlug(cfg.TenantSlug)
	if tenantSlug == "" {
		return errors.New("tenant slug required")
	}
	provider := normalizeProvider(cfg.Provider)
	if provider == "" {
		return errors.New("invalid provider")
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		return errors.New("model required")
	}
	apiKey := strings.TrimSpace(cfg.APIKey)
	headersJSON := strings.TrimSpace(cfg.HeadersJSON)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(`
INSERT INTO tenant_inference_config(
  tenant_slug, provider, base_url, model, api_key, headers_json, updated_at
) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(tenant_slug) DO UPDATE SET
  provider = excluded.provider,
  base_url = excluded.base_url,
  model = excluded.model,
  api_key = excluded.api_key,
  headers_json = excluded.headers_json,
  updated_at = excluded.updated_at
`, tenantSlug, provider, baseURL, model, apiKey, headersJSON, now)
	if err != nil {
		return fmt.Errorf("upsert inference config: %w", err)
	}
	return nil
}


package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/inference"
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type inferConfigDTO struct {
	Provider string            `json:"provider"`
	BaseURL  string            `json:"base_url"`
	Model    string            `json:"model"`
	APIKey   string            `json:"api_key,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
}

func (s *Server) handleInferConfigGet(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	cfg, err := s.control.TenantInferenceConfig(tenant.Slug)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := inferConfigDTO{}
	if cfg != nil {
		out.Provider = cfg.Provider
		out.BaseURL = cfg.BaseURL
		out.Model = cfg.Model
		// Deliberately do not return api_key.
		if strings.TrimSpace(cfg.HeadersJSON) != "" {
			_ = json.Unmarshal([]byte(cfg.HeadersJSON), &out.Headers)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleInferConfigPost(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var dto inferConfigDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// If api_key is omitted/blank, preserve the existing key (if any).
	if strings.TrimSpace(dto.APIKey) == "" {
		if existing, err := s.control.TenantInferenceConfig(tenant.Slug); err == nil && existing != nil {
			dto.APIKey = existing.APIKey
		}
	}

	headersJSON := ""
	if len(dto.Headers) > 0 {
		if b, err := json.Marshal(dto.Headers); err == nil {
			headersJSON = string(b)
		}
	}
	if err := s.control.UpsertTenantInferenceConfig(control.InferenceConfig{
		TenantSlug:  tenant.Slug,
		Provider:    dto.Provider,
		BaseURL:     dto.BaseURL,
		Model:       dto.Model,
		APIKey:      dto.APIKey,
		HeadersJSON: headersJSON,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type inferStreamRequest struct {
	Messages []inference.Message `json:"messages"`
}

func (s *Server) handleInferStream(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 512<<10))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var req inferStreamRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	cfg, err := s.control.TenantInferenceConfig(tenant.Slug)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if cfg == nil {
		http.Error(w, "missing inference config for tenant", http.StatusBadRequest)
		return
	}
	headers := map[string]string{}
	if strings.TrimSpace(cfg.HeadersJSON) != "" {
		_ = json.Unmarshal([]byte(cfg.HeadersJSON), &headers)
	}

	client, err := inference.New(inference.Config{
		Provider: inference.ProviderKind(cfg.Provider),
		BaseURL:  cfg.BaseURL,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		Headers:  headers,
		Timeout:  2 * time.Minute,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	bw := bufio.NewWriterSize(w, 16<<10)
	flush := func() {
		_ = bw.Flush()
		if flusher != nil {
			flusher.Flush()
		}
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	_, _ = client.Stream(ctx, inference.Request{Messages: req.Messages}, func(ev inference.StreamEvent) error {
		// Emit OpenAI-style SSE: event + data.
		if strings.TrimSpace(ev.Type) != "" {
			_, _ = bw.WriteString("event: " + strings.TrimSpace(ev.Type) + "\n")
		}
		if len(ev.Data) > 0 {
			_, _ = bw.WriteString("data: " + strings.TrimSpace(string(ev.Data)) + "\n\n")
		} else {
			_, _ = bw.WriteString("data: {}\n\n")
		}
		flush()
		return nil
	})
}

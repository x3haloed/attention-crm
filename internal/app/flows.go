package app

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

func (s *Server) storeFlow(flow ceremonyFlow) string {
	token := make([]byte, 24)
	_, _ = rand.Read(token)
	id := base64.RawURLEncoding.EncodeToString(token)

	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	now := time.Now().UTC()
	for key, existing := range s.webauthnFlow {
		if now.After(existing.ExpiresAt) {
			delete(s.webauthnFlow, key)
		}
	}
	s.webauthnFlow[id] = flow
	return id
}

func (s *Server) consumeFlow(flowID string) (ceremonyFlow, bool) {
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	flow, ok := s.webauthnFlow[flowID]
	if !ok {
		return ceremonyFlow{}, false
	}
	delete(s.webauthnFlow, flowID)
	if time.Now().UTC().After(flow.ExpiresAt) {
		return ceremonyFlow{}, false
	}
	return flow, true
}

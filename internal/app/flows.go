package app

import (
	"encoding/base64"
	"fmt"
	"time"
)

func (s *Server) storeFlow(flow ceremonyFlow) (string, error) {
	token := make([]byte, 24)
	if _, err := cryptoRandRead(token); err != nil {
		return "", fmt.Errorf("could not generate flow token: %w", err)
	}
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
	return id, nil
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

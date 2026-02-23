package app

import (
	"net/http"
)

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"version":  BuildVersion,
		"commit":   BuildCommit,
		"built_at": BuildTime,
	})
}

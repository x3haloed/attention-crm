package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

func parseMaybeMultipartForm(r *http.Request) error {
	// Our auth flows submit using fetch(FormData), which defaults to multipart/form-data.
	// Calling ParseForm first will *not* parse multipart bodies but will initialize r.Form,
	// which prevents FormValue from later calling ParseMultipartForm.
	// Keep maxMemory small; total request size is already capped by MaxBytesReader middleware.
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			return r.ParseForm()
		}
		return err
	}
	return nil
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	id := requestID(r)
	if err != nil {
		log.Printf("internal_error rid=%s method=%s path=%s err=%v", id, r.Method, r.URL.Path, err)
	} else {
		log.Printf("internal_error rid=%s method=%s path=%s", id, r.Method, r.URL.Path)
	}
	http.Error(w, "internal error (request_id="+id+")", http.StatusInternalServerError)
}

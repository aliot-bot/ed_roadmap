package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"roadmap/backend/internal/apperrors"
)

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	de := json.NewDecoder(r.Body)
	de.DisallowUnknownFields()

	if err := de.Decode(v); err != nil {
		writeError(w, 400, "invalid request", nil)
		return false
	}
	if err := de.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, 400, "invalid request", nil)
		return false
	}
	return true
}

func idParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, e := uuid.Parse(chi.URLParam(r, "id"))
	if e != nil {
		writeError(w, 400, "invalid id", nil)
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string, details any) {
	writeJSON(w, status, map[string]any{
		"message": msg,
		"details": details,
	})
}

func respondErr(w http.ResponseWriter, e error) {
	mapped := apperrors.HTTP(e)
	writeError(w, mapped.Status, mapped.Message, mapped.Details)
}

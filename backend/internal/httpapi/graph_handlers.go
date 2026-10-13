package httpapi

import (
	"net/http"

	"roadmap/backend/internal/domain"
)

func (a *API) dependencies(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.Dependencies(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}

func (a *API) setDependencies(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Dependencies []domain.Dependency `json:"dependencies"`
	}
	if !decode(w, r, &v) {
		return
	}
	if e := a.app.SetDependencies(r.Context(), principal(r), id, v.Dependencies); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}

func (a *API) progress(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.Progress(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, v)
}

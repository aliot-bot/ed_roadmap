package httpapi

import (
	"net/http"

	"roadmap/backend/internal/domain"
)

func (a *API) layouts(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.Layouts(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}

func (a *API) setLayouts(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Items []domain.Layout `json:"items"`
	}
	if !decode(w, r, &v) {
		return
	}
	if e := a.app.SetLayouts(r.Context(), principal(r), id, v.Items); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}

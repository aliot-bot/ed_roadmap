package httpapi

import (
	"net/http"

	"roadmap/backend/internal/domain"
)

func (a *API) note(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.GetNote(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, v)
}

func (a *API) setNote(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v domain.Note
	if !decode(w, r, &v) {
		return
	}
	if e := a.app.SetNote(r.Context(), principal(r), id, v.Content); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}

func (a *API) checklist(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	items, err := a.app.Checklist(r.Context(), principal(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) setChecklist(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Items []domain.ChecklistItemInput `json:"items"`
	}
	if !decode(w, r, &v) {
		return
	}
	items, err := a.app.SetChecklist(r.Context(), principal(r), id, v.Items)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) document(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.GetDocument(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, v)
}

func (a *API) setDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Content string `json:"content"`
	}
	if !decode(w, r, &v) {
		return
	}
	d, e := a.app.SetDocument(r.Context(), principal(r), id, v.Content)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, d)
}

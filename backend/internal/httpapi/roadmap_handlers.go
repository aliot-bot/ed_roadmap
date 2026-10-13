package httpapi

import (
	"fmt"
	"io"
	"net/http"
)

func (a *API) listRoadmaps(w http.ResponseWriter, r *http.Request) {
	v, e := a.app.ListRoadmaps(r.Context(), principal(r))
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}

func (a *API) getRoadmap(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.GetRoadmap(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"roadmap": v})
}

func (a *API) createRoadmap(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decode(w, r, &v) {
		return
	}
	out, e := a.app.CreateRoadmap(r.Context(), principal(r), v.Title, v.Description)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 201, out)
}

func (a *API) updateRoadmap(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if !decode(w, r, &v) {
		return
	}
	out, e := a.app.UpdateRoadmap(r.Context(), principal(r), id, v.Title, v.Description)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, out)
}

func (a *API) deleteRoadmap(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if e := a.app.DeleteRoadmap(r.Context(), principal(r), id); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}

func (a *API) roadmapCover(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	cover, err := a.app.GetRoadmapCover(r.Context(), principal(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}
	w.Header().Set("Content-Type", cover.MIMEType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(cover.Data)))
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cover.Data)
}

func (a *API) setRoadmapCover(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 6*1024*1024)
	if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
		writeError(w, http.StatusBadRequest, "invalid image upload", nil)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "image file is required", nil)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 5*1024*1024+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid image upload", nil)
		return
	}

	mime := detectRoadmapCoverMIME(data)
	cover, err := a.app.SetRoadmapCover(r.Context(), principal(r), id, mime, data)
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mime_type": cover.MIMEType, "updated_at": cover.UpdatedAt})
}

func (a *API) deleteRoadmapCover(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if err := a.app.DeleteRoadmapCover(r.Context(), principal(r), id); err != nil {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func detectRoadmapCoverMIME(data []byte) string {
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) == 0 {
		return ""
	}
	detected := http.DetectContentType(data[:min(len(data), 512)])
	switch detected {
	case "image/jpeg", "image/png", "image/gif":
		return detected
	default:
		return ""
	}
}

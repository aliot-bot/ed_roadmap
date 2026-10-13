package httpapi

import (
	"io"
	"net/http"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *API) uploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10*1024*1024 + 1024); err != nil {
		writeError(w, 400, "invalid upload", nil)
		return
	}

	topic, err := uuid.Parse(r.FormValue("topic_id"))
	if err != nil {
		writeError(w, 400, "topic_id is required", nil)
		return
	}

	f, h, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "file is required", nil)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(http.MaxBytesReader(w, f, 10*1024*1024+1))
	if err != nil {
		writeError(w, 400, "invalid upload", nil)
		return
	}

	mime := h.Header.Get("Content-Type")
	x, err := a.app.SaveImage(r.Context(), principal(r), topic, mime, data)
	if err != nil {
		respondErr(w, err)
		return
	}

	writeJSON(w, 201, x)
}

func (a *API) image(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}

	x, err := a.app.GetImage(r.Context(), principal(r), id)
	if err != nil {
		respondErr(w, err)
		return
	}

	w.Header().Set("Content-Type", x.MIMEType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(200)
	_, _ = w.Write(x.Data)
}

func (a *API) learningActive(w http.ResponseWriter, r *http.Request) {
	v, e := a.app.ActiveLearning(r.Context(), principal(r))
	if e != nil {
		respondErr(w, e)
		return
	}

	writeJSON(w, 200, map[string]any{"session": v})
}

func (a *API) learningSessions(w http.ResponseWriter, r *http.Request) {
	from, to, _, e := learningRange(r)
	if e != nil {
		respondErr(w, e)
		return
	}

	v, e := a.app.LearningSessions(r.Context(), principal(r), from, to)
	if e != nil {
		respondErr(w, e)
		return
	}

	writeJSON(w, 200, map[string]any{"items": v})
}

func (a *API) learningStart(w http.ResponseWriter, r *http.Request) {
	var v struct {
		ElementID uuid.UUID `json:"elementId"`
	}
	if !decode(w, r, &v) || v.ElementID == uuid.Nil {
		writeError(w, 400, "elementId is required", nil)
		return
	}

	out, e := a.app.StartLearningElement(r.Context(), principal(r), v.ElementID)
	if e != nil {
		respondErr(w, e)
		return
	}

	writeJSON(w, 201, out)
}

func (a *API) learningTransition(w http.ResponseWriter, r *http.Request, status string) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}

	var v struct {
		Note *string `json:"note"`
	}
	if r.ContentLength != 0 && !decode(w, r, &v) {
		return
	}

	out, e := a.app.TransitionLearning(r.Context(), principal(r), id, status, v.Note)
	if e != nil {
		respondErr(w, e)
		return
	}

	writeJSON(w, 200, out)
}

func (a *API) learningPause(w http.ResponseWriter, r *http.Request) {
	a.learningTransition(w, r, domain.LearningPaused)
}

func (a *API) learningResume(w http.ResponseWriter, r *http.Request) {
	a.learningTransition(w, r, domain.LearningActive)
}

func (a *API) learningFinish(w http.ResponseWriter, r *http.Request) {
	a.learningTransition(w, r, domain.LearningFinished)
}

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"roadmap/backend/internal/domain"
)

func (a *API) analytics(w http.ResponseWriter, r *http.Request) {
	v, e := a.app.Analytics(r.Context(), principal(r))
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, v)
}

func learningRange(r *http.Request) (time.Time, time.Time, *time.Location, error) {
	loc := time.UTC
	if name := r.URL.Query().Get("timezone"); name != "" {
		loaded, err := time.LoadLocation(name)
		if err != nil {
			return time.Time{}, time.Time{}, time.UTC, domain.ValidationError{Field: "timezone", Reason: "unknown timezone"}
		}
		loc = loaded
	}

	parse := func(raw, field string) (time.Time, error) {
		if t, e := time.Parse(time.RFC3339, raw); e == nil {
			return t, nil
		}
		if d, e := time.ParseInLocation("2006-01-02", raw, loc); e == nil {
			return d, nil
		}
		return time.Time{}, domain.ValidationError{Field: field, Reason: "must be RFC3339 or YYYY-MM-DD"}
	}

	to := time.Now().UTC()
	from := to.Add(-30 * 24 * time.Hour)
	if raw := r.URL.Query().Get("from"); raw != "" {
		var err error
		from, err = parse(raw, "from")
		if err != nil {
			return time.Time{}, time.Time{}, time.UTC, domain.ValidationError{Field: "from", Reason: "must be RFC3339"}
		}
	}

	if raw := r.URL.Query().Get("to"); raw != "" {
		var err error
		to, err = parse(raw, "to")
		if err != nil {
			return time.Time{}, time.Time{}, time.UTC, domain.ValidationError{Field: "to", Reason: "must be RFC3339"}
		}
	}

	if !to.After(from) {
		return time.Time{}, time.Time{}, time.UTC, domain.ValidationError{Field: "range", Reason: "to must be after from"}
	}
	return from, to, loc, nil
}

func (a *API) learningOverview(w http.ResponseWriter, r *http.Request) {
	from, to, loc, e := learningRange(r)
	if e != nil {
		respondErr(w, e)
		return
	}
	v, e := a.app.LearningOverview(r.Context(), principal(r), from, to, loc)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, v)
}

func (a *API) learningActivity(w http.ResponseWriter, r *http.Request) {
	from, to, loc, e := learningRange(r)
	if e != nil {
		respondErr(w, e)
		return
	}
	v, e := a.app.LearningActivity(r.Context(), principal(r), from, to, loc)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}

func (a *API) learningActivityDate(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	parts, err := time.Parse("2006-01-02", date)
	if err != nil {
		writeError(w, 400, "invalid date", nil)
		return
	}

	loc := time.UTC
	if z := r.URL.Query().Get("timezone"); z != "" {
		loc, err = time.LoadLocation(z)
		if err != nil {
			writeError(w, 400, "invalid timezone", nil)
			return
		}
	}

	from := time.Date(parts.Year(), parts.Month(), parts.Day(), 0, 0, 0, 0, loc)
	to := from.AddDate(0, 0, 1)
	items, e := a.app.LearningActivity(r.Context(), principal(r), from.UTC(), to.UTC(), loc)
	if e != nil {
		respondErr(w, e)
		return
	}

	if len(items) == 0 {
		writeJSON(w, 200, map[string]any{"day": nil})
		return
	}
	writeJSON(w, 200, map[string]any{"day": items[0]})
}

func (a *API) learningAchievements(w http.ResponseWriter, r *http.Request) {
	from, to, loc, e := learningRange(r)

	if r.URL.Query().Get("from") == "" && r.URL.Query().Get("to") == "" {
		from = time.Unix(0, 0).UTC()
	}
	if e != nil {
		respondErr(w, e)
		return
	}

	v, e := a.app.LearningAchievements(r.Context(), principal(r), from, to, loc)
	if e != nil {
		respondErr(w, e)
		return
	}
	items := []map[string]any{
		{
			"kind": "4h",
			"days": v.FourHours,
		},
		{
			"kind": "6h",
			"days": v.SixHours,
		},
		{
			"kind": "8h",
			"days": v.EightHours,
		},
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (a *API) learningAnalysisLatest(w http.ResponseWriter, r *http.Request) {
	v, e := a.app.LatestLearningReport(r.Context(), principal(r))
	if errors.Is(e, domain.ErrNotFound) {
		writeJSON(w, 200, map[string]any{"report": nil})
		return
	}
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"report": v})
}

func (a *API) learningAnalysis(w http.ResponseWriter, r *http.Request) {
	from, to, loc, e := learningRange(r)
	if e != nil {
		respondErr(w, e)
		return
	}
	v, e := a.app.AnalyzeLearning(r.Context(), principal(r), from, to, loc)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, v)
}

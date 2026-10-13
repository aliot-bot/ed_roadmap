package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/security"
	"roadmap/backend/internal/service"
)

type Config struct {
	SecureCookies   bool
	CookieSameSite  http.SameSite
	MCPEnabled      bool
	MCPToken        string
	MCPOwnerLogin   string
	MCPScopes       []string
	AccessKeyPepper string
	AllowBootstrap  bool
}
type API struct {
	app         *service.App
	cfg         Config
	logger      *slog.Logger
	authLimiter *limiter
}

func New(app *service.App, cfg Config, logger *slog.Logger) *API {
	if cfg.CookieSameSite == 0 {
		cfg.CookieSameSite = http.SameSiteLaxMode
	}
	if logger == nil {
		logger = slog.Default()
	}
	app.SetAccessKeyPepper(cfg.AccessKeyPepper)
	return &API{app: app, cfg: cfg, logger: logger, authLimiter: newLimiter(5, time.Minute)}
}

type contextKey uint8

const principalKey contextKey = 1

type sessionAuth struct {
	principal domain.Principal
	csrfHash  []byte
	token     string
}

func authFrom(ctx context.Context) (sessionAuth, bool) {
	v, ok := ctx.Value(principalKey).(sessionAuth)
	return v, ok
}

func (a *API) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(a.recoverer, a.securityHeaders)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Route("/api", func(r chi.Router) {
		if a.cfg.AllowBootstrap || a.cfg.AccessKeyPepper == "" {
			r.Post("/auth/bootstrap", a.loginLimited(a.bootstrap))
		}
		r.Post("/auth/login", a.loginLimited(a.login))
		r.Post("/auth/key-login", a.loginLimited(a.keyLogin))
		r.Group(func(r chi.Router) {
			r.Use(a.session)
			r.Get("/auth/me", a.me)
			r.Post("/auth/logout", a.csrf(a.logout))
			r.Get("/roadmaps", a.listRoadmaps)
			r.Post("/roadmaps", a.csrf(a.createRoadmap))
			r.Get("/roadmaps/{id}", a.getRoadmap)
			r.Patch("/roadmaps/{id}", a.csrf(a.updateRoadmap))
			r.Delete("/roadmaps/{id}", a.csrf(a.deleteRoadmap))
			r.Get("/roadmaps/{id}/cover", a.roadmapCover)
			r.Put("/roadmaps/{id}/cover", a.csrf(a.setRoadmapCover))
			r.Delete("/roadmaps/{id}/cover", a.csrf(a.deleteRoadmapCover))
			r.Get("/roadmaps/{id}/topics", a.listTopics)
			r.Post("/roadmaps/{id}/topics", a.csrf(a.createTopic))
			r.Get("/roadmaps/{id}/dependencies", a.dependencies)
			r.Put("/roadmaps/{id}/dependencies", a.csrf(a.setDependencies))
			r.Get("/roadmaps/{id}/progress", a.progress)
			r.Get("/roadmaps/{id}/layout", a.layouts)
			r.Put("/roadmaps/{id}/layout", a.csrf(a.setLayouts))
			r.Get("/analytics", a.analytics)
			r.Get("/learning/overview", a.learningOverview)
			r.Get("/learning/activity", a.learningActivity)
			r.Get("/learning/activity/{date}", a.learningActivityDate)
			r.Get("/learning/achievements", a.learningAchievements)
			r.Get("/learning/analysis/latest", a.learningAnalysisLatest)
			r.Post("/learning/analysis", a.csrf(a.learningAnalysis))
			r.Post("/uploads/images", a.csrf(a.uploadImage))
			r.Get("/uploads/images/{id}", a.image)
			r.Get("/learning/sessions/active", a.learningActive)
			r.Get("/learning/sessions", a.learningSessions)
			r.Post("/learning/sessions/start", a.csrf(a.learningStart))
			r.Post("/learning/sessions/{id}/pause", a.csrf(a.learningPause))
			r.Post("/learning/sessions/{id}/resume", a.csrf(a.learningResume))
			r.Post("/learning/sessions/{id}/finish", a.csrf(a.learningFinish))
			r.Patch("/topics/{id}", a.csrf(a.updateTopic))
			r.Delete("/topics/{id}", a.csrf(a.deleteTopic))
			r.Put("/topics/{id}/completion", a.csrf(a.setCompletion))
			r.Put("/topics/{id}/status", a.csrf(a.setTopicStatus))
			r.Get("/topics/{id}/note", a.note)
			r.Put("/topics/{id}/note", a.csrf(a.setNote))
			r.Get("/topics/{id}/checklist", a.checklist)
			r.Put("/topics/{id}/checklist", a.csrf(a.setChecklist))
			r.Get("/topics/{id}/document", a.document)
			r.Put("/topics/{id}/document", a.csrf(a.setDocument))
		})
	})
	if a.cfg.MCPEnabled {
		r.Handle("/mcp", a.mcpAuth(http.HandlerFunc(a.mcp)))
	}
	return r
}
func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				writeError(w, http.StatusInternalServerError, "internal server error", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func (a *API) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func (a *API) session(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("roadmap_session")
		if err != nil {
			writeError(w, 401, "authentication required", nil)
			return
		}
		p, csrf, err := a.app.AuthenticateSession(r.Context(), c.Value)
		if err != nil {
			writeError(w, 401, "authentication required", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, sessionAuth{p, csrf, c.Value})))
	})
}
func (a *API) csrf(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := authFrom(r.Context())
		if !ok || subtle.ConstantTimeCompare(security.TokenHash(r.Header.Get("X-CSRF-Token")), s.csrfHash) != 1 {
			writeError(w, 403, "invalid csrf token", nil)
			return
		}
		next(w, r)
	}
}
func (a *API) mcpAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if auth == "" || a.cfg.MCPToken == "" || subtle.ConstantTimeCompare([]byte(auth), []byte(a.cfg.MCPToken)) != 1 {
			writeError(w, 401, "mcp authentication required", nil)
			return
		}
		p, err := a.app.MCPPrincipal(r.Context(), a.cfg.MCPOwnerLogin, a.cfg.MCPScopes)
		if err != nil {
			writeError(w, 403, "mcp authorization unavailable", nil)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, sessionAuth{principal: p})))
	})
}
func principal(r *http.Request) domain.Principal { s, _ := authFrom(r.Context()); return s.principal }

func (a *API) bootstrap(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	out, err := a.app.Bootstrap(r.Context(), v.Login, v.Password)
	if err != nil {
		respondErr(w, err)
		return
	}
	a.loginResponse(w, out)
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	out, err := a.app.Login(r.Context(), v.Login, v.Password)
	if err != nil {
		respondErr(w, err)
		return
	}
	a.loginResponse(w, out)
}
func (a *API) keyLogin(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &v) {
		return
	}
	out, err := a.app.LoginWithKey(r.Context(), v.Key)
	if err != nil {
		writeError(w, 401, "invalid access key", nil)
		return
	}
	a.loginResponse(w, out)
}
func (a *API) loginResponse(w http.ResponseWriter, out service.LoginResult) {
	http.SetCookie(w, &http.Cookie{Name: "roadmap_session", Value: out.SessionToken, Path: "/", Expires: out.ExpiresAt, HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: a.cfg.CookieSameSite})
	http.SetCookie(w, &http.Cookie{Name: "roadmap_csrf", Value: out.CSRFToken, Path: "/", Expires: out.ExpiresAt, Secure: a.cfg.SecureCookies, SameSite: a.cfg.CookieSameSite})
	w.Header().Set("X-CSRF-Token", out.CSRFToken)
	writeJSON(w, 200, map[string]any{"user": out.User})
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.app.User(r.Context(), principal(r))
	if err != nil {
		respondErr(w, err)
		return
	}
	// The CSRF secret is intentionally not persisted in plaintext. The browser's
	// non-HttpOnly SameSite cookie is accepted only when its hash matches this
	// authenticated session, then echoed for the SPA after a page reload.
	if c, err := r.Cookie("roadmap_csrf"); err == nil {
		s, _ := authFrom(r.Context())
		if subtle.ConstantTimeCompare(security.TokenHash(c.Value), s.csrfHash) == 1 {
			w.Header().Set("X-CSRF-Token", c.Value)
		}
	}
	writeJSON(w, 200, map[string]any{"user": u})
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	s, _ := authFrom(r.Context())
	_ = a.app.Logout(r.Context(), s.token)
	http.SetCookie(w, &http.Cookie{Name: "roadmap_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.cfg.SecureCookies, SameSite: a.cfg.CookieSameSite})
	http.SetCookie(w, &http.Cookie{Name: "roadmap_csrf", Value: "", Path: "/", MaxAge: -1, Secure: a.cfg.SecureCookies, SameSite: a.cfg.CookieSameSite})
	w.WriteHeader(http.StatusNoContent)
}
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
func (a *API) listTopics(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	v, e := a.app.ListTopics(r.Context(), principal(r), id)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, map[string]any{"items": v})
}
func (a *API) createTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v domain.TopicInput
	if !decode(w, r, &v) {
		return
	}
	out, e := a.app.CreateTopic(r.Context(), principal(r), id, v)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 201, out)
}
func (a *API) updateTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if !decode(w, r, &raw) {
		return
	}
	var in service.TopicPatch
	var e error
	if b, ok := raw["parent_id"]; ok {
		in.ParentSet = true
		if string(b) != "null" {
			var x uuid.UUID
			e = json.Unmarshal(b, &x)
			in.ParentID = &x
		}
	}
	if b, ok := raw["title"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Title = &x
	}
	if b, ok := raw["description"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Description = &x
	}
	if b, ok := raw["sort_order"]; ok {
		var x int
		e = json.Unmarshal(b, &x)
		in.SortOrder = &x
	}
	if b, ok := raw["kind"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Kind = &x
	}
	if b, ok := raw["task_status"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.TaskStatus = &x
	}
	if b, ok := raw["status"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Status = &x
	}
	if b, ok := raw["priority"]; ok {
		var x string
		e = json.Unmarshal(b, &x)
		in.Priority = &x
	}
	if e != nil {
		writeError(w, 400, "invalid request", nil)
		return
	}
	out, e := a.app.UpdateTopic(r.Context(), principal(r), id, in)
	if e != nil {
		respondErr(w, e)
		return
	}
	writeJSON(w, 200, out)
}
func (a *API) setTopicStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &v) || v.Status == "" {
		writeError(w, 400, "status is required", nil)
		return
	}
	if err := a.app.SetTopicStatus(r.Context(), principal(r), id, v.Status); err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"status": v.Status})
}
func (a *API) deleteTopic(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	if e := a.app.DeleteTopic(r.Context(), principal(r), id); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}
func (a *API) setCompletion(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var v struct {
		Completed *bool `json:"completed"`
	}
	if !decode(w, r, &v) || v.Completed == nil {
		if v.Completed == nil {
			writeError(w, 400, "completed is required", nil)
		}
		return
	}
	if e := a.app.SetCompletion(r.Context(), principal(r), id, *v.Completed); e != nil {
		respondErr(w, e)
		return
	}
	w.WriteHeader(204)
}
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
	items := []map[string]any{{"kind": "4h", "days": v.FourHours}, {"kind": "6h", "days": v.SixHours}, {"kind": "8h", "days": v.EightHours}}
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
	writeJSON(w, status, map[string]any{"message": msg, "details": details})
}
func respondErr(w http.ResponseWriter, e error) {
	var v domain.ValidationError
	switch {
	case errors.As(e, &v):
		writeError(w, 400, "validation failed", v)
	case errors.Is(e, domain.ErrUnauthorized):
		writeError(w, 401, "authentication required", nil)
	case errors.Is(e, domain.ErrForbidden):
		writeError(w, 403, "access denied", nil)
	case errors.Is(e, domain.ErrNotFound):
		writeError(w, 404, "not found", nil)
	case errors.Is(e, domain.ErrConflict):
		writeError(w, 409, "conflict", nil)
	default:
		writeError(w, 500, "internal server error", nil)
	}
}

type limiter struct {
	mu     sync.Mutex
	limit  int
	period time.Duration
	visits map[string][]time.Time
}

func newLimiter(limit int, period time.Duration) *limiter {
	return &limiter{limit: limit, period: period, visits: map[string][]time.Time{}}
}
func (a *API) loginLimited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
		if err != nil || len(body) > 1<<20 {
			writeError(w, 400, "invalid request", nil)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Login string `json:"login"`
		}
		_ = json.Unmarshal(body, &request)
		if !a.authLimiter.allow(loginLimitKey(r, request.Login)) {
			w.Header().Set("Retry-After", "60")
			writeError(w, 429, "too many requests", nil)
			return
		}
		next(w, r)
	}
}
func loginLimitKey(r *http.Request, login string) string {
	candidate := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if candidate == "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			candidate = host
		} else {
			candidate = r.RemoteAddr
		}
	}
	ip := net.ParseIP(candidate)
	normalizedIP := "unknown"
	if ip != nil {
		normalizedIP = ip.String()
	}
	return normalizedIP + ":" + strings.ToLower(strings.TrimSpace(login))
}
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cut := now.Add(-l.period)
	old := l.visits[key]
	keep := old[:0]
	for _, t := range old {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= l.limit {
		l.visits[key] = keep
		return false
	}
	l.visits[key] = append(keep, now)
	return true
}

func rpcError(w http.ResponseWriter, id any, code int, msg string) {
	writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}
func (a *API) mcp(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(405)
		return
	}
	var q struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if !decode(w, r, &q) {
		return
	}
	if q.JSONRPC != "2.0" {
		rpcError(w, q.ID, -32600, "invalid request")
		return
	}
	switch q.Method {
	case "initialize":
		version := "2025-03-26" // retained for clients using the initial MVP handshake.
		var init struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(q.Params, &init) == nil && init.ProtocolVersion >= "2026-07-28" {
			version = "2026-07-28"
		}
		w.Header().Set("MCP-Protocol-Version", version)
		w.Header().Set("Mcp-Method", q.Method)
		w.Header().Set("Mcp-Name", "knowledge-roadmap")
		writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "knowledge-roadmap", "version": "0.1.0"}}})
	case "tools/list":
		writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": q.ID, "result": map[string]any{"tools": mcpTools()}})
	case "tools/call":
		a.callTool(w, r, q.ID, q.Params)
	default:
		rpcError(w, q.ID, -32601, "method not found")
	}
}
func mcpTools() []map[string]any {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	uuidSchema := map[string]any{"type": "string", "format": "uuid"}
	idempotency := map[string]any{"type": "string", "minLength": 8, "maxLength": 200}
	priority := map[string]any{"type": "string", "enum": []string{"critical", "important", "optional", "skippable"}}
	topic := object(map[string]any{"client_ref": map[string]any{"type": "string", "maxLength": 100}, "parent_id": uuidSchema, "parent_ref": map[string]any{"type": "string", "maxLength": 100}, "title": map[string]any{"type": "string", "minLength": 1, "maxLength": 300}, "description": map[string]any{"type": "string", "maxLength": 20000}, "sort_order": map[string]any{"type": "integer"}, "priority": priority}, "title")
	topicOutput := map[string]any{"type": "object", "properties": map[string]any{"id": uuidSchema, "roadmap_id": uuidSchema, "title": map[string]any{"type": "string"}, "status": map[string]any{"type": "string", "enum": []string{"not_started", "in_progress", "needs_review", "completed"}}, "priority": priority, "kind": map[string]any{"type": "string"}}}
	dependency := object(map[string]any{"predecessor_topic_id": uuidSchema, "successor_topic_id": uuidSchema}, "predecessor_topic_id", "successor_topic_id")
	checklistItem := object(map[string]any{"text": map[string]any{"type": "string", "minLength": 1, "maxLength": 500}}, "text")
	return []map[string]any{
		{"name": "list_roadmaps", "description": "List owned roadmaps", "inputSchema": object(map[string]any{})},
		{"name": "get_roadmap", "description": "Get roadmap metadata, topics, dependencies and progress", "inputSchema": object(map[string]any{"roadmap_id": uuidSchema}, "roadmap_id"), "outputSchema": map[string]any{"type": "object", "properties": map[string]any{"topics": map[string]any{"type": "array", "items": topicOutput}}}},
		{"name": "get_learning_statistics", "description": "Read confirmed timer aggregates; unsupported metrics are null", "inputSchema": object(map[string]any{"from": map[string]any{"type": "string"}, "to": map[string]any{"type": "string"}, "timezone": map[string]any{"type": "string"}})},
		{"name": "get_learning_activity", "description": "Read confirmed timer activity by local day", "inputSchema": object(map[string]any{"from": map[string]any{"type": "string"}, "to": map[string]any{"type": "string"}, "timezone": map[string]any{"type": "string"}})},
		{"name": "create_roadmap", "description": "Create a roadmap; requires idempotency_key", "inputSchema": object(map[string]any{"idempotency_key": idempotency, "title": map[string]any{"type": "string", "minLength": 1, "maxLength": 200}, "description": map[string]any{"type": "string", "maxLength": 5000}}, "idempotency_key", "title")},
		{"name": "create_topics_batch", "description": "Atomically create 1-200 topics; requires idempotency_key", "inputSchema": object(map[string]any{"idempotency_key": idempotency, "roadmap_id": uuidSchema, "topics": map[string]any{"type": "array", "minItems": 1, "maxItems": 200, "items": topic}}, "idempotency_key", "roadmap_id", "topics")},
		{"name": "update_topic", "description": "Update structural topic fields; requires idempotency_key", "inputSchema": object(map[string]any{"idempotency_key": idempotency, "topic_id": uuidSchema, "title": map[string]any{"type": "string", "minLength": 1, "maxLength": 300}, "description": map[string]any{"type": "string", "maxLength": 20000}, "parent_id": map[string]any{"anyOf": []any{uuidSchema, map[string]any{"type": "null"}}}, "sort_order": map[string]any{"type": "integer"}}, "idempotency_key", "topic_id")},
		{"name": "set_topic_priority", "description": "Set a topic or task priority; requires idempotency_key", "inputSchema": object(map[string]any{"idempotency_key": idempotency, "topic_id": uuidSchema, "priority": priority}, "idempotency_key", "topic_id", "priority")},
		{"name": "set_dependencies_batch", "description": "Atomically replace dependencies; requires idempotency_key", "inputSchema": object(map[string]any{"idempotency_key": idempotency, "roadmap_id": uuidSchema, "dependencies": map[string]any{"type": "array", "maxItems": 4000, "items": dependency}}, "idempotency_key", "roadmap_id", "dependencies")},
		{"name": "add_checklist_items", "description": "Add unchecked items to a topic checklist without replacing existing items; requires idempotency_key", "inputSchema": object(map[string]any{"idempotency_key": idempotency, "topic_id": uuidSchema, "items": map[string]any{"type": "array", "minItems": 1, "maxItems": 80, "items": checklistItem}}, "idempotency_key", "topic_id", "items")},
	}
}
func toolResult(w http.ResponseWriter, id any, result any) {
	writeJSON(w, 200, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  map[string]any{"content": []map[string]any{{"type": "text", "text": mustJSON(result)}}, "isError": false, "resultType": "complete"},
	})
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func (a *API) callTool(w http.ResponseWriter, r *http.Request, id any, raw json.RawMessage) {
	var c struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		rpcError(w, id, -32602, "invalid tool call")
		return
	}
	p := principal(r)
	var out any
	var err error
	switch c.Name {
	case "list_roadmaps":
		var x struct{}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil {
			out, err = a.app.ListRoadmaps(r.Context(), p)
		}
	case "get_roadmap":
		var x struct {
			RoadmapID uuid.UUID `json:"roadmap_id"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.RoadmapID == uuid.Nil {
			err = domain.ValidationError{Field: "roadmap_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.roadmapDetail(r.Context(), p, x.RoadmapID)
		}
	case "get_learning_statistics", "get_learning_activity":
		var x struct {
			From     string `json:"from"`
			To       string `json:"to"`
			Timezone string `json:"timezone"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil {
			from, to := time.Now().UTC().Add(-30*24*time.Hour), time.Now().UTC()
			if x.From != "" {
				from, err = time.Parse(time.RFC3339, x.From)
			}
			if err == nil && x.To != "" {
				to, err = time.Parse(time.RFC3339, x.To)
			}
			loc := time.UTC
			if err == nil && x.Timezone != "" {
				loc, err = time.LoadLocation(x.Timezone)
			}
			if err != nil {
				err = domain.ValidationError{Field: "range", Reason: "invalid RFC3339 range or timezone"}
			} else if c.Name == "get_learning_statistics" {
				out, err = a.app.LearningOverview(r.Context(), p, from, to, loc)
			} else {
				out, err = a.app.LearningActivity(r.Context(), p, from, to, loc)
			}
		}
	case "create_roadmap":
		var x struct {
			IdempotencyKey string `json:"idempotency_key"`
			Title          string `json:"title"`
			Description    string `json:"description"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil {
			out, err = a.app.Idempotent(r.Context(), p, x.IdempotencyKey, x, func() (any, error) { return a.app.CreateRoadmap(r.Context(), p, x.Title, x.Description) })
		}
	case "create_topics_batch":
		var x struct {
			IdempotencyKey string              `json:"idempotency_key"`
			RoadmapID      uuid.UUID           `json:"roadmap_id"`
			Topics         []domain.TopicInput `json:"topics"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.RoadmapID == uuid.Nil {
			err = domain.ValidationError{Field: "roadmap_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.app.Idempotent(r.Context(), p, x.IdempotencyKey, x, func() (any, error) { return a.app.CreateTopicsBatch(r.Context(), p, x.RoadmapID, x.Topics) })
		}
	case "update_topic":
		var x struct {
			IdempotencyKey string     `json:"idempotency_key"`
			TopicID        uuid.UUID  `json:"topic_id"`
			Title          *string    `json:"title"`
			Description    *string    `json:"description"`
			ParentID       *uuid.UUID `json:"parent_id"`
			SortOrder      *int       `json:"sort_order"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.TopicID == uuid.Nil {
			err = domain.ValidationError{Field: "topic_id", Reason: "is required"}
		}
		if err == nil {
			patch := service.TopicPatch{Title: x.Title, Description: x.Description, SortOrder: x.SortOrder}
			if bytesPresent(c.Arguments, "parent_id") {
				patch.ParentSet = true
				patch.ParentID = x.ParentID
			}
			out, err = a.app.Idempotent(r.Context(), p, x.IdempotencyKey, x, func() (any, error) { return a.app.UpdateTopic(r.Context(), p, x.TopicID, patch) })
		}
	case "set_dependencies_batch":
		var x struct {
			IdempotencyKey string              `json:"idempotency_key"`
			RoadmapID      uuid.UUID           `json:"roadmap_id"`
			Dependencies   []domain.Dependency `json:"dependencies"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.RoadmapID == uuid.Nil {
			err = domain.ValidationError{Field: "roadmap_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.app.Idempotent(r.Context(), p, x.IdempotencyKey, x, func() (any, error) {
				return map[string]bool{"ok": true}, a.app.SetDependencies(r.Context(), p, x.RoadmapID, x.Dependencies)
			})
		}
	case "set_topic_priority":
		var x struct {
			IdempotencyKey string    `json:"idempotency_key"`
			TopicID        uuid.UUID `json:"topic_id"`
			Priority       string    `json:"priority"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.TopicID == uuid.Nil {
			err = domain.ValidationError{Field: "topic_id", Reason: "is required"}
		}
		if err == nil {
			out, err = a.app.Idempotent(r.Context(), p, x.IdempotencyKey, x, func() (any, error) {
				return map[string]bool{"ok": true}, a.app.SetTopicPriority(r.Context(), p, x.TopicID, x.Priority)
			})
		}
	case "add_checklist_items":
		var x struct {
			IdempotencyKey string    `json:"idempotency_key"`
			TopicID        uuid.UUID `json:"topic_id"`
			Items          []struct {
				Text string `json:"text"`
			} `json:"items"`
		}
		err = decodeMCPArguments(c.Arguments, &x)
		if err == nil && x.TopicID == uuid.Nil {
			err = domain.ValidationError{Field: "topic_id", Reason: "is required"}
		}
		if err == nil {
			texts := make([]string, len(x.Items))
			for i, item := range x.Items {
				texts[i] = item.Text
			}
			out, err = a.app.Idempotent(r.Context(), p, x.IdempotencyKey, x, func() (any, error) {
				return a.app.AppendChecklist(r.Context(), p, x.TopicID, texts)
			})
		}
	default:
		rpcError(w, id, -32601, "unknown tool")
		return
	}
	if err != nil {
		a.logger.Warn("mcp tool failed", "tool", c.Name, "error", err)
		rpcError(w, id, -32602, safeToolError(err))
		return
	}
	toolResult(w, id, out)
}
func decodeMCPArguments(raw json.RawMessage, value any) error {
	if len(raw) == 0 {
		return domain.ValidationError{Field: "arguments", Reason: "is required"}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return domain.ValidationError{Field: "arguments", Reason: "must match the tool input schema"}
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.ValidationError{Field: "arguments", Reason: "must contain one object"}
	}
	return nil
}
func (a *API) roadmapDetail(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.RoadmapDetail, error) {
	r, err := a.app.GetRoadmap(ctx, p, id)
	if err != nil {
		return domain.RoadmapDetail{}, err
	}
	topics, err := a.app.ListTopics(ctx, p, id)
	if err != nil {
		return domain.RoadmapDetail{}, err
	}
	deps, err := a.app.Dependencies(ctx, p, id)
	if err != nil {
		return domain.RoadmapDetail{}, err
	}
	return domain.RoadmapDetail{Roadmap: r, Topics: topics, Dependencies: deps, Progress: service.CalculateProgress(topics)}, nil
}
func bytesPresent(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m[key] != nil
}
func safeToolError(err error) string {
	if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrUnauthorized) {
		return "access denied"
	}
	if errors.Is(err, domain.ErrNotFound) {
		return "not found"
	}
	var v domain.ValidationError
	if errors.As(err, &v) {
		return fmt.Sprintf("invalid input: %s", v.Error())
	}
	if errors.Is(err, domain.ErrConflict) {
		return "conflict"
	}
	return "operation failed"
}

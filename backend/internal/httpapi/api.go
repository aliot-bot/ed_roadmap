package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
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
	app         Application
	cfg         Config
	logger      Logger
	authLimiter *limiter
}

type nopLogger struct{}

func (nopLogger) Warn(string, ...any) {}

func New(app Application, cfg Config, logger Logger) *API {
	if cfg.CookieSameSite == 0 {
		cfg.CookieSameSite = http.SameSiteLaxMode
	}
	if configurator, ok := app.(accessKeyPepperConfigurator); ok {
		configurator.SetAccessKeyPepper(cfg.AccessKeyPepper)
	}
	if logger == nil {
		logger = nopLogger{}
	}
	return &API{
		app:         app,
		cfg:         cfg,
		logger:      logger,
		authLimiter: newLimiter(5, time.Minute),
	}
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

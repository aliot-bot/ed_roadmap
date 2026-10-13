package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/security"
)

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

func (a *API) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				a.logger.Warn("http panic recovered", "operation", "http_request", "error_class", "internal")
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
		auth := sessionAuth{
			principal: p,
			csrfHash:  csrf,
			token:     c.Value,
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, auth)))
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
		authSession := sessionAuth{principal: p}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, authSession)))
	})
}

func principal(r *http.Request) domain.Principal {
	s, _ := authFrom(r.Context())
	return s.principal
}

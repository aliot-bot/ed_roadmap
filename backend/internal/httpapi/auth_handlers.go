package httpapi

import (
	"crypto/subtle"
	"net/http"

	"roadmap/backend/internal/security"
	"roadmap/backend/internal/service"
)

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
	http.SetCookie(w, &http.Cookie{
		Name:     "roadmap_session",
		Value:    out.SessionToken,
		Path:     "/",
		Expires:  out.ExpiresAt,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: a.cfg.CookieSameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "roadmap_csrf",
		Value:    out.CSRFToken,
		Path:     "/",
		Expires:  out.ExpiresAt,
		Secure:   a.cfg.SecureCookies,
		SameSite: a.cfg.CookieSameSite,
	})

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

	http.SetCookie(w, &http.Cookie{
		Name:     "roadmap_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.SecureCookies,
		SameSite: a.cfg.CookieSameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "roadmap_csrf",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   a.cfg.SecureCookies,
		SameSite: a.cfg.CookieSameSite,
	})

	w.WriteHeader(http.StatusNoContent)
}

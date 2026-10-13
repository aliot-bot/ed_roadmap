package config

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(lookup(map[string]string{"DATABASE_URL": "postgres://db"}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Database.MaxConns != 10 || cfg.HTTP.Addr != ":8080" || cfg.Session.TTL != 14*24*time.Hour {
		t.Fatalf("unexpected core defaults: %#v", cfg)
	}
	if !cfg.MCP.Enabled || cfg.MCP.MaxBatchTopics != 200 || !cfg.Cookies.Secure || cfg.Cookies.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected protocol defaults: %#v", cfg)
	}
	if cfg.MCP.Token != "" || cfg.MCP.OwnerLogin != "" || cfg.AllowBootstrap || len(cfg.MCP.Scopes) != 2 {
		t.Fatalf("unexpected optional defaults: %#v", cfg)
	}
}

func TestLoadPrecedenceAndParsing(t *testing.T) {
	cfg, err := Load(lookup(map[string]string{
		"DATABASE_URL":       "postgres://db",
		"DATABASE_MAX_CONNS": "12",
		"SESSION_TTL":        "3h",
		"MCP_STATIC_TOKEN":   "static",
		"MCP_BEARER_TOKEN":   "bearer",
		"OWNER_LOGIN":        "owner",
		"MCP_OWNER_LOGIN":    "fallback-owner",
		"MCP_SCOPES":         " roadmaps:read, ,roadmaps:write ",
		"COOKIE_SAME_SITE":   "strict",
		"COOKIE_SECURE":      "false",
		"MCP_ENABLED":        "0",
		"ALLOW_BOOTSTRAP":    "yes",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MCP.Token != "static" || cfg.MCP.OwnerLogin != "owner" || cfg.Cookies.SameSite != http.SameSiteStrictMode {
		t.Fatalf("precedence failed: %#v", cfg)
	}
	if cfg.Cookies.Secure || cfg.MCP.Enabled || !cfg.AllowBootstrap || cfg.Database.MaxConns != 12 || cfg.Session.TTL != 3*time.Hour {
		t.Fatalf("parsing failed: %#v", cfg)
	}
	if strings.Join(cfg.MCP.Scopes, ",") != "roadmaps:read,roadmaps:write" {
		t.Fatalf("scopes = %#v", cfg.MCP.Scopes)
	}
}

func TestLoadFallbacksAndRequiredDatabase(t *testing.T) {
	values := map[string]string{
		"DATABASE_MAX_CONNS": "0",
		"SESSION_TTL":        "not-a-duration",
		"COOKIE_SAME_SITE":   "unknown",
	}
	if _, err := Load(lookup(values)); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("missing URL error = %v", err)
	}
	values["DATABASE_URL"] = "postgres://db"
	cfg, err := Load(lookup(values))
	if err != nil {
		t.Fatalf("Load() fallback error = %v", err)
	}
	if cfg.Database.MaxConns != 10 || cfg.Session.TTL != 14*24*time.Hour || cfg.Cookies.SameSite != http.SameSiteLaxMode {
		t.Fatalf("fallbacks failed: %#v", cfg)
	}
}

func lookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

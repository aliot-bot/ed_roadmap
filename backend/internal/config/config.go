package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultDatabaseMaxConns = 10
	defaultHTTPAddr         = ":8080"
	defaultSessionTTL       = 14 * 24 * time.Hour
	defaultMCPBatchTopics   = 200
	defaultConnectTimeout   = 20 * time.Second
	defaultReadHeader       = 5 * time.Second
	defaultReadTimeout      = 15 * time.Second
	defaultWriteTimeout     = 30 * time.Second
	defaultIdleTimeout      = 60 * time.Second
	defaultShutdownTimeout  = 15 * time.Second
)

const DefaultHealthcheckTimeout = 2 * time.Second

type Config struct {
	Database        DatabaseConfig
	HTTP            HTTPConfig
	Session         SessionConfig
	MCP             MCPConfig
	Cookies         CookieConfig
	AccessKeyPepper string
	AllowBootstrap  bool
}

type DatabaseConfig struct {
	URL            string
	MaxConns       int
	ConnectTimeout time.Duration
}

type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type SessionConfig struct {
	TTL time.Duration
}

type MCPConfig struct {
	Enabled        bool
	Token          string
	OwnerLogin     string
	Scopes         []string
	MaxBatchTopics int
}

type CookieConfig struct {
	Secure   bool
	SameSite http.SameSite
}

func Load(lookup func(string) string) (Config, error) {
	if lookup == nil {
		lookup = os.Getenv
	}

	cfg := Config{
		Database: DatabaseConfig{
			URL:            lookup("DATABASE_URL"),
			MaxConns:       positiveInt(lookup("DATABASE_MAX_CONNS"), defaultDatabaseMaxConns),
			ConnectTimeout: defaultConnectTimeout,
		},
		HTTP: HTTPConfig{
			Addr:              valueOr(lookup("HTTP_ADDR"), defaultHTTPAddr),
			ReadHeaderTimeout: defaultReadHeader,
			ReadTimeout:       defaultReadTimeout,
			WriteTimeout:      defaultWriteTimeout,
			IdleTimeout:       defaultIdleTimeout,
			ShutdownTimeout:   defaultShutdownTimeout,
		},
		Session: SessionConfig{
			TTL: positiveDuration(lookup("SESSION_TTL"), defaultSessionTTL),
		},
		MCP: MCPConfig{
			Enabled:        boolValue(lookup("MCP_ENABLED"), true),
			Token:          firstValue(lookup, "MCP_STATIC_TOKEN", "MCP_BEARER_TOKEN"),
			OwnerLogin:     firstValue(lookup, "OWNER_LOGIN", "MCP_OWNER_LOGIN"),
			Scopes:         split(lookup("MCP_SCOPES")),
			MaxBatchTopics: positiveInt(lookup("MCP_MAX_BATCH_TOPICS"), defaultMCPBatchTopics),
		},
		Cookies: CookieConfig{
			Secure:   boolValue(lookup("COOKIE_SECURE"), true),
			SameSite: sameSite(lookup("COOKIE_SAME_SITE")),
		},
		AccessKeyPepper: lookup("ACCESS_KEY_PEPPER"),
		AllowBootstrap:  boolValue(lookup("ALLOW_BOOTSTRAP"), false),
	}

	if len(cfg.MCP.Scopes) == 0 {
		cfg.MCP.Scopes = []string{"roadmaps:read", "roadmaps:write"}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.Database.MaxConns <= 0 || c.Database.ConnectTimeout <= 0 {
		return fmt.Errorf("database configuration is invalid")
	}
	if c.HTTP.Addr == "" || c.HTTP.ReadHeaderTimeout <= 0 || c.HTTP.ReadTimeout <= 0 || c.HTTP.WriteTimeout <= 0 || c.HTTP.IdleTimeout <= 0 || c.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("http configuration is invalid")
	}
	if c.Session.TTL <= 0 {
		return fmt.Errorf("session configuration is invalid")
	}
	if c.MCP.MaxBatchTopics <= 0 {
		return fmt.Errorf("mcp configuration is invalid")
	}
	if c.Cookies.SameSite != http.SameSiteLaxMode && c.Cookies.SameSite != http.SameSiteStrictMode && c.Cookies.SameSite != http.SameSiteNoneMode {
		return fmt.Errorf("cookie configuration is invalid")
	}
	return nil
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func positiveInt(raw string, fallback int) int {
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err == nil && value > 0 {
		return value
	}
	return fallback
}

func positiveDuration(raw string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boolValue(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func firstValue(lookup func(string) string, keys ...string) string {
	for _, key := range keys {
		if value := lookup(key); value != "" {
			return value
		}
	}
	return ""
}

func split(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func sameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

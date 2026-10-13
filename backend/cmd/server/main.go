package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"roadmap/backend/internal/httpapi"
	"roadmap/backend/internal/migrations"
	"roadmap/backend/internal/repository"
	"roadmap/backend/internal/security"
	"roadmap/backend/internal/service"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthcheck()
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		logger.Error("parse database config", "error", err)
		os.Exit(1)
	}
	poolConfig.MaxConns = int32(intEnv("DATABASE_MAX_CONNS", 10))
	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}
	if err := migrations.Apply(ctx, db); err != nil {
		logger.Error("apply migrations", "error", err)
		os.Exit(1)
	}
	scopes := split(os.Getenv("MCP_SCOPES"))
	if len(scopes) == 0 {
		scopes = []string{"roadmaps:read", "roadmaps:write"}
	}
	store := repository.NewPostgres(db)
	if len(os.Args) > 1 && os.Args[1] == "users" {
		if err := usersCommand(ctx, store, os.Args[2:], os.Getenv("ACCESS_KEY_PEPPER")); err != nil {
			logger.Error("users command failed", "error", err)
			os.Exit(1)
		}
		return
	}
	app := service.NewWithLimits(store, durationEnv("SESSION_TTL", 14*24*time.Hour), intEnv("MCP_MAX_BATCH_TOPICS", 200))
	api := httpapi.New(app, httpapi.Config{SecureCookies: envBool("COOKIE_SECURE", true), CookieSameSite: cookieSameSite(os.Getenv("COOKIE_SAME_SITE")), MCPEnabled: envBool("MCP_ENABLED", true), MCPToken: envFirst("MCP_STATIC_TOKEN", "MCP_BEARER_TOKEN"), MCPOwnerLogin: envFirst("OWNER_LOGIN", "MCP_OWNER_LOGIN"), MCPScopes: scopes, AccessKeyPepper: os.Getenv("ACCESS_KEY_PEPPER"), AllowBootstrap: envBool("ALLOW_BOOTSTRAP", false)}, logger)
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: api.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		logger.Info("server started", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdown); err != nil {
		logger.Error("graceful shutdown", "error", err)
	}
}
func usersCommand(ctx context.Context, store *repository.Postgres, args []string, pepper string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: users create|rotate-key|revoke-key ...")
	}
	command, login := args[0], args[1]
	if command == "create" {
		display := ""
		if len(args) > 2 {
			display = args[2]
		}
		key, err := security.RandomToken("km_")
		if err != nil {
			return err
		}
		_, err = store.CreateUserWithAccessKey(ctx, login, display, "", security.AccessKeyHash(key, pepper))
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stdout, key)
		return nil
	}
	u, _, err := store.FindUserByLogin(ctx, login)
	if err != nil {
		return err
	}
	switch command {
	case "rotate-key":
		key, e := security.RandomToken("km_")
		if e != nil {
			return e
		}
		if e = store.RotateAccessKey(ctx, u.ID, security.AccessKeyHash(key, pepper)); e != nil {
			return e
		}
		fmt.Fprintln(os.Stdout, key)
		return nil
	case "revoke-key":
		return store.RevokeAccessKey(ctx, u.ID)
	default:
		return fmt.Errorf("unknown users command")
	}
}
func healthcheck() {
	c := http.Client{Timeout: 2 * time.Second}
	r, err := c.Get("http://127.0.0.1:8080/healthz")
	if err != nil || r.StatusCode != http.StatusOK {
		if r != nil {
			r.Body.Close()
		}
		os.Exit(1)
	}
	r.Body.Close()
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func envBool(key string, fallback bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes"
}
func split(v string) []string {
	var out []string
	for _, x := range strings.Split(v, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}
func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}
func durationEnv(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}
func intEnv(key string, fallback int) int {
	var n int
	if _, err := fmt.Sscanf(os.Getenv(key), "%d", &n); err == nil && n > 0 {
		return n
	}
	return fallback
}
func cookieSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

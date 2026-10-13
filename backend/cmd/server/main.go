package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"roadmap/backend/internal/apperrors"
	"roadmap/backend/internal/clog"
	"roadmap/backend/internal/config"
	"roadmap/backend/internal/domain"
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
	logger := clog.NewJSON(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		logger.Error("configuration failed", "operation", "config_load", "error_class", apperrors.Class(err))
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Database.ConnectTimeout)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		logger.Error("database configuration failed", "operation", "database_config", "error_class", "internal")
		os.Exit(1)
	}
	poolConfig.MaxConns = int32(cfg.Database.MaxConns)

	db, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Error("database connection failed", "operation", "database_connect", "error_class", "internal")
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		logger.Error("database ping failed", "operation", "database_ping", "error_class", "internal")
		os.Exit(1)
	}

	if err := migrations.Apply(ctx, db); err != nil {
		logger.Error("database migrations failed", "operation", "migrations", "error_class", "internal")
		os.Exit(1)
	}

	store := repository.NewPostgres(db)
	if len(os.Args) > 1 && os.Args[1] == "users" {
		if err := usersCommand(ctx, store, os.Args[2:], cfg.AccessKeyPepper); err != nil {
			logger.Error("users command failed", "operation", "users_command", "error_class", apperrors.Class(err))
			os.Exit(1)
		}
		return
	}

	app := service.NewWithLimits(store, cfg.Session.TTL, cfg.MCP.MaxBatchTopics)
	api := httpapi.New(app, httpapi.Config{
		SecureCookies:   cfg.Cookies.Secure,
		CookieSameSite:  cfg.Cookies.SameSite,
		MCPEnabled:      cfg.MCP.Enabled,
		MCPToken:        cfg.MCP.Token,
		MCPOwnerLogin:   cfg.MCP.OwnerLogin,
		MCPScopes:       cfg.MCP.Scopes,
		AccessKeyPepper: cfg.AccessKeyPepper,
		AllowBootstrap:  cfg.AllowBootstrap,
	}, logger)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           api.Router(),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	go func() {
		logger.Info("server started", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "operation", "http_server", "error_class", "internal")
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	shutdown, cancelShutdown := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdown); err != nil {
		logger.Error("graceful shutdown failed", "operation", "shutdown", "error_class", "internal")
	}
}

type usersStore interface {
	CreateUserWithAccessKey(context.Context, string, string, string, []byte) (domain.User, error)
	FindUserByLogin(context.Context, string) (domain.User, string, error)
	RotateAccessKey(context.Context, uuid.UUID, []byte) error
	RevokeAccessKey(context.Context, uuid.UUID) error
}

func usersCommand(ctx context.Context, store usersStore, args []string, pepper string) error {
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
	c := http.Client{Timeout: config.DefaultHealthcheckTimeout}
	r, err := c.Get("http://127.0.0.1:8080/healthz")
	if err != nil || r.StatusCode != http.StatusOK {
		if r != nil {
			r.Body.Close()
		}
		os.Exit(1)
	}
	r.Body.Close()
}

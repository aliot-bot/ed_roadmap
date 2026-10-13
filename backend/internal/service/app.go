package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/security"
)

const (
	maxTopicsPerRoadmap   = 2000
	defaultMaxBatchTopics = 200
	maxDependencies       = 4000
	maxLayoutItems        = 2000
	maxTopicDocumentChars = 200000
	maxChecklistItems     = 80
	maxChecklistItemChars = 500
	maxRoadmapCoverBytes  = 5 * 1024 * 1024
)

type App struct {
	store           Store
	sessionTTL      time.Duration
	maxBatchTopics  int
	accessKeyPepper string
	reportProvider  LearningReportProvider
}

type LearningReportProvider interface {
	Analyze(context.Context, domain.LearningOverview, []domain.LearningActivityDay) (string, error)
}

func (a *App) SetLearningReportProvider(p LearningReportProvider) {
	a.reportProvider = p
}

func New(store Store) *App {
	return NewWithLimits(store, 14*24*time.Hour, defaultMaxBatchTopics)
}

func NewWithLimits(store Store, sessionTTL time.Duration, maxBatchTopics int) *App {
	if sessionTTL <= 0 {
		sessionTTL = 14 * 24 * time.Hour
	}
	if maxBatchTopics < 1 || maxBatchTopics > 1000 {
		maxBatchTopics = defaultMaxBatchTopics
	}
	return &App{
		store:          store,
		sessionTTL:     sessionTTL,
		maxBatchTopics: maxBatchTopics,
	}
}

func (a *App) SetAccessKeyPepper(pepper string) {
	a.accessKeyPepper = pepper
}

type LoginResult struct {
	User         domain.User
	SessionToken string
	CSRFToken    string
	ExpiresAt    time.Time
}

func (a *App) Bootstrap(ctx context.Context, login, password string) (LoginResult, error) {
	if err := validateLogin(login); err != nil {
		return LoginResult{}, err
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return LoginResult{}, err
	}
	u, err := a.store.BootstrapUser(ctx, strings.TrimSpace(login), hash)
	if err != nil {
		return LoginResult{}, err
	}
	return a.newSession(ctx, u)
}

func validateLogin(login string) error {
	return validateText("login", login, 3, 254)
}

func (a *App) Login(ctx context.Context, login, password string) (LoginResult, error) {
	u, hash, err := a.store.FindUserByLogin(ctx, strings.TrimSpace(login))
	if err != nil || !security.CheckPassword(hash, password) {
		return LoginResult{}, domain.ErrUnauthorized
	}
	return a.newSession(ctx, u)
}

func (a *App) LoginWithKey(ctx context.Context, key string) (LoginResult, error) {
	s, ok := a.store.(AccessKeyStore)
	if !ok || strings.TrimSpace(key) == "" {
		return LoginResult{}, domain.ErrUnauthorized
	}
	u, err := s.FindUserByAccessKey(ctx, security.AccessKeyHash(key, a.accessKeyPepper))
	if err != nil {
		return LoginResult{}, domain.ErrUnauthorized
	}
	return a.newSession(ctx, u)
}

func (a *App) newSession(ctx context.Context, u domain.User) (LoginResult, error) {
	token, err := security.RandomToken("s_")
	if err != nil {
		return LoginResult{}, err
	}
	csrf, err := security.RandomToken("c_")
	if err != nil {
		return LoginResult{}, err
	}
	expires := time.Now().UTC().Add(a.sessionTTL)
	err = a.store.CreateSession(ctx, domain.Session{
		ID:        uuid.New(),
		UserID:    u.ID,
		TokenHash: security.TokenHash(token),
		CSRFHash:  security.TokenHash(csrf),
		ExpiresAt: expires,
	})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{
		User:         u,
		SessionToken: token,
		CSRFToken:    csrf,
		ExpiresAt:    expires,
	}, nil
}

func (a *App) AuthenticateSession(ctx context.Context, token string) (domain.Principal, []byte, error) {
	if token == "" {
		return domain.Principal{}, nil, domain.ErrUnauthorized
	}
	s, err := a.store.FindSession(ctx, security.TokenHash(token), time.Now().UTC())
	if err != nil {
		return domain.Principal{}, nil, domain.ErrUnauthorized
	}
	return domain.Principal{
		UserID: s.UserID,
		Scopes: map[string]bool{
			domain.ScopeRead:  true,
			domain.ScopeWrite: true,
		},
		Actor: "browser",
	}, s.CSRFHash, nil
}

func (a *App) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return a.store.DeleteSession(ctx, security.TokenHash(token))
}

func (a *App) User(ctx context.Context, p domain.Principal) (domain.User, error) {
	return a.store.FindUser(ctx, p.UserID)
}

func read(p domain.Principal) error {
	if !p.Can(domain.ScopeRead) {
		return domain.ErrForbidden
	}
	return nil
}

func write(p domain.Principal) error {
	if !p.Can(domain.ScopeWrite) {
		return domain.ErrForbidden
	}
	return nil
}

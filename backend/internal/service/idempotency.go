package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"roadmap/backend/internal/domain"
)

// Idempotent executes an MCP write exactly once for a principal and operation key.
// The stored JSON result is replayed only when the same request payload is sent.
func (a *App) Idempotent(ctx context.Context, p domain.Principal, key string, request any, fn func() (any, error)) (any, error) {
	if len(key) < 8 || len(key) > 200 {
		return nil, domain.ValidationError{Field: "idempotency_key", Reason: "must contain 8-200 characters"}
	}
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if saved, ok, err := a.store.GetIdempotency(ctx, p.UserID, key, sum[:]); err != nil {
		return nil, err
	} else if ok {
		var out any
		if err := json.Unmarshal(saved, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	out, err := fn()
	if err != nil {
		return nil, err
	}
	saved, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if err = a.store.PutIdempotency(ctx, p.UserID, key, sum[:], saved); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("record idempotency: %w", err)
	}
	return out, nil
}

func (a *App) MCPPrincipal(ctx context.Context, login string, scopes []string) (domain.Principal, error) {
	u, _, err := a.store.FindUserByLogin(ctx, login)
	if err != nil {
		return domain.Principal{}, err
	}
	set := map[string]bool{}
	for _, s := range scopes {
		if s == domain.ScopeRead || s == domain.ScopeWrite {
			set[s] = true
		}
	}
	if len(set) == 0 {
		return domain.Principal{}, domain.ErrForbidden
	}
	return domain.Principal{UserID: u.ID, Scopes: set, Actor: "mcp"}, nil
}

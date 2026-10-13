package service

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *App) Layouts(ctx context.Context, p domain.Principal, roadmap uuid.UUID) ([]domain.Layout, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	return a.store.ListLayouts(ctx, p.UserID, roadmap)
}

func (a *App) SetLayouts(ctx context.Context, p domain.Principal, roadmap uuid.UUID, items []domain.Layout) error {
	if err := write(p); err != nil {
		return err
	}
	if len(items) > maxLayoutItems {
		return domain.ValidationError{Field: "items", Reason: "too many layout items"}
	}
	return a.store.ReplaceLayouts(ctx, p.UserID, roadmap, items)
}

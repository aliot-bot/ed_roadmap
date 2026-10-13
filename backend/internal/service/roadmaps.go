package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *App) ListRoadmaps(ctx context.Context, p domain.Principal) ([]domain.Roadmap, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	roadmaps, err := a.store.ListRoadmaps(ctx, p.UserID)
	if err != nil {
		return nil, err
	}
	for i := range roadmaps {
		topics, err := a.store.ListTopics(ctx, p.UserID, roadmaps[i].ID)
		if err != nil {
			return nil, err
		}
		progress := CalculateProgress(topics)
		roadmaps[i].Progress = &progress
	}
	return roadmaps, nil
}

func (a *App) GetRoadmap(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.Roadmap, error) {
	if err := read(p); err != nil {
		return domain.Roadmap{}, err
	}
	return a.store.GetRoadmap(ctx, p.UserID, id)
}

func (a *App) CreateRoadmap(ctx context.Context, p domain.Principal, title, desc string) (domain.Roadmap, error) {
	if err := write(p); err != nil {
		return domain.Roadmap{}, err
	}
	if err := validateText("title", title, 1, 200); err != nil {
		return domain.Roadmap{}, err
	}
	if err := validateText("description", desc, 0, 5000); err != nil {
		return domain.Roadmap{}, err
	}
	r, err := a.store.CreateRoadmap(ctx, p.UserID, strings.TrimSpace(title), desc)
	if err == nil {
		_ = a.store.Audit(ctx, p.UserID, p.Actor, "create", "roadmap", &r.ID, nil)
	}
	return r, err
}

func (a *App) UpdateRoadmap(ctx context.Context, p domain.Principal, id uuid.UUID, title, desc *string) (domain.Roadmap, error) {
	if err := write(p); err != nil {
		return domain.Roadmap{}, err
	}
	if title == nil && desc == nil {
		return domain.Roadmap{}, domain.ValidationError{Field: "body", Reason: "at least one field is required"}
	}
	if title != nil {
		if err := validateText("title", *title, 1, 200); err != nil {
			return domain.Roadmap{}, err
		}
		v := strings.TrimSpace(*title)
		title = &v
	}
	if desc != nil {
		if err := validateText("description", *desc, 0, 5000); err != nil {
			return domain.Roadmap{}, err
		}
	}
	r, err := a.store.UpdateRoadmap(ctx, p.UserID, id, title, desc)
	if err == nil {
		_ = a.store.Audit(ctx, p.UserID, p.Actor, "update", "roadmap", &id, nil)
	}
	return r, err
}

func (a *App) DeleteRoadmap(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	if err := write(p); err != nil {
		return err
	}
	if err := a.store.DeleteRoadmap(ctx, p.UserID, id); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "delete", "roadmap", &id, nil)
}

func (a *App) GetRoadmapCover(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.RoadmapCover, error) {
	if err := read(p); err != nil {
		return domain.RoadmapCover{}, err
	}
	return a.store.GetRoadmapCover(ctx, p.UserID, id)
}

func (a *App) SetRoadmapCover(ctx context.Context, p domain.Principal, id uuid.UUID, mime string, data []byte) (domain.RoadmapCover, error) {
	if err := write(p); err != nil {
		return domain.RoadmapCover{}, err
	}
	if err := validateRoadmapCover(mime, data); err != nil {
		return domain.RoadmapCover{}, err
	}
	cover, err := a.store.SetRoadmapCover(ctx, p.UserID, id, mime, data)
	if err != nil {
		return domain.RoadmapCover{}, err
	}
	_ = a.store.Audit(ctx, p.UserID, p.Actor, "cover", "roadmap", &id, map[string]any{"mime_type": mime, "bytes": len(data)})
	return cover, nil
}

func (a *App) DeleteRoadmapCover(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	if err := write(p); err != nil {
		return err
	}
	if err := a.store.DeleteRoadmapCover(ctx, p.UserID, id); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "cover_delete", "roadmap", &id, nil)
}

func validateRoadmapCover(mime string, data []byte) error {
	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}
	if !allowed[mime] {
		return domain.ValidationError{Field: "mime_type", Reason: "must be JPEG, PNG, GIF, or WebP"}
	}
	if len(data) == 0 || len(data) > maxRoadmapCoverBytes {
		return domain.ValidationError{Field: "file", Reason: "must be between 1 byte and 5 MB"}
	}
	return nil
}

package service

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *App) GetNote(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.Note, error) {
	if err := read(p); err != nil {
		return domain.Note{}, err
	}
	return a.store.GetNote(ctx, p.UserID, id)
}

func (a *App) SetNote(ctx context.Context, p domain.Principal, id uuid.UUID, content string) error {
	if err := write(p); err != nil {
		return err
	}
	if err := validateText("content", content, 0, 100000); err != nil {
		return err
	}
	if err := a.store.SetNote(ctx, p.UserID, id, content); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "note", "topic", &id, nil)
}

func (a *App) Checklist(ctx context.Context, p domain.Principal, id uuid.UUID) ([]domain.ChecklistItem, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	return a.store.ListChecklist(ctx, p.UserID, id)
}

func (a *App) SetChecklist(ctx context.Context, p domain.Principal, id uuid.UUID, items []domain.ChecklistItemInput) ([]domain.ChecklistItem, error) {
	if err := write(p); err != nil {
		return nil, err
	}
	if len(items) > maxChecklistItems {
		return nil, domain.ValidationError{Field: "items", Reason: "must contain at most 80 items"}
	}
	seen := make(map[uuid.UUID]bool)
	for _, item := range items {
		if err := validateText("text", item.Text, 1, maxChecklistItemChars); err != nil {
			return nil, err
		}
		if item.ID != nil {
			if seen[*item.ID] {
				return nil, domain.ValidationError{Field: "items", Reason: "item IDs must be unique"}
			}
			seen[*item.ID] = true
		}
	}
	out, err := a.store.SetChecklist(ctx, p.UserID, id, items)
	if err != nil {
		return nil, err
	}
	_ = a.store.Audit(ctx, p.UserID, p.Actor, "checklist", "topic", &id, map[string]any{"count": len(out)})
	return out, nil
}

// AppendChecklist adds new unchecked items while preserving the user's existing checklist.
func (a *App) AppendChecklist(ctx context.Context, p domain.Principal, id uuid.UUID, texts []string) ([]domain.ChecklistItem, error) {
	if err := write(p); err != nil {
		return nil, err
	}
	if len(texts) == 0 || len(texts) > maxChecklistItems {
		return nil, domain.ValidationError{Field: "items", Reason: "must contain 1 to 80 items"}
	}
	for _, text := range texts {
		if err := validateText("text", text, 1, maxChecklistItemChars); err != nil {
			return nil, err
		}
	}
	out, err := a.store.AppendChecklist(ctx, p.UserID, id, texts)
	if err != nil {
		return nil, err
	}
	_ = a.store.Audit(ctx, p.UserID, p.Actor, "checklist_append", "topic", &id, map[string]any{"count": len(out)})
	return out, nil
}

func (a *App) GetDocument(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.TopicDocument, error) {
	if err := read(p); err != nil {
		return domain.TopicDocument{}, err
	}
	return a.store.GetTopicDocument(ctx, p.UserID, id)
}

func (a *App) SetDocument(ctx context.Context, p domain.Principal, id uuid.UUID, content string) (domain.TopicDocument, error) {
	if err := write(p); err != nil {
		return domain.TopicDocument{}, err
	}
	// Markdown whitespace is meaningful (for example in fenced code blocks), so
	// enforce the storage limit against the untrimmed Unicode content.
	if len([]rune(content)) > maxTopicDocumentChars {
		return domain.TopicDocument{}, domain.ValidationError{Field: "content", Reason: "must contain at most 200000 characters"}
	}
	d, err := a.store.SetTopicDocument(ctx, p.UserID, id, content)
	if err != nil {
		return domain.TopicDocument{}, err
	}
	_ = a.store.Audit(ctx, p.UserID, p.Actor, "document", "topic", &id, nil)
	return d, nil
}

func (a *App) Dependencies(ctx context.Context, p domain.Principal, roadmap uuid.UUID) ([]domain.Dependency, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	return a.store.ListDependencies(ctx, p.UserID, roadmap)
}

func (a *App) SetDependencies(ctx context.Context, p domain.Principal, roadmap uuid.UUID, deps []domain.Dependency) error {
	if err := write(p); err != nil {
		return err
	}
	if len(deps) > maxDependencies {
		return domain.ValidationError{Field: "dependencies", Reason: "too many dependencies"}
	}
	topics, err := a.store.ListTopics(ctx, p.UserID, roadmap)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, len(topics))
	for i, t := range topics {
		ids[i] = t.ID
	}
	if err := ValidateDependencies(ids, deps); err != nil {
		return err
	}
	if err := a.store.ReplaceDependencies(ctx, p.UserID, roadmap, deps); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "set_dependencies", "roadmap", &roadmap, map[string]any{"count": len(deps)})
}

func (a *App) Progress(ctx context.Context, p domain.Principal, roadmap uuid.UUID) (domain.Progress, error) {
	topics, err := a.ListTopics(ctx, p, roadmap)
	if err != nil {
		return domain.Progress{}, err
	}
	return CalculateProgress(topics), nil
}

package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *App) ListTopics(ctx context.Context, p domain.Principal, roadmap uuid.UUID) ([]domain.Topic, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	return a.store.ListTopics(ctx, p.UserID, roadmap)
}

func validateTopic(title, desc string) error {
	if err := validateText("title", title, 1, 300); err != nil {
		return err
	}
	return validateText("description", desc, 0, 20000)
}

func validateTopicKind(kind, status string) error {
	if kind == "" {
		kind = domain.ElementTopic
	}
	if kind != domain.ElementTopic && kind != domain.ElementTask {
		return domain.ValidationError{Field: "kind", Reason: "must be topic or task"}
	}
	if status == "" {
		status = domain.TaskNotStarted
	}
	if status != domain.TaskNotStarted && status != domain.TaskInProgress && status != domain.TaskCompleted {
		return domain.ValidationError{Field: "task_status", Reason: "invalid task status"}
	}
	if kind == domain.ElementTopic && status != domain.TaskNotStarted {
		return domain.ValidationError{Field: "task_status", Reason: "topic status must be not_started"}
	}
	return nil
}

func validateTopicStatus(status string) error {
	if status == "" {
		return nil
	}
	if status != domain.TopicNotStarted && status != domain.TopicInProgress && status != domain.TopicNeedsReview && status != domain.TopicCompleted {
		return domain.ValidationError{Field: "status", Reason: "invalid topic status"}
	}
	return nil
}

func validatePriority(priority string) error {
	if priority == "" {
		return nil
	}
	if priority != domain.PriorityCritical && priority != domain.PriorityImportant && priority != domain.PriorityOptional && priority != domain.PrioritySkippable {
		return domain.ValidationError{Field: "priority", Reason: "invalid priority"}
	}
	return nil
}

func (a *App) CreateTopic(ctx context.Context, p domain.Principal, roadmap uuid.UUID, in domain.TopicInput) (domain.Topic, error) {
	if err := write(p); err != nil {
		return domain.Topic{}, err
	}
	if err := validateTopic(in.Title, in.Description); err != nil {
		return domain.Topic{}, err
	}
	if err := validateTopicKind(in.Kind, in.TaskStatus); err != nil {
		return domain.Topic{}, err
	}
	if err := validateTopicStatus(in.Status); err != nil {
		return domain.Topic{}, err
	}
	if err := validatePriority(in.Priority); err != nil {
		return domain.Topic{}, err
	}
	topics, err := a.store.ListTopics(ctx, p.UserID, roadmap)
	if err != nil {
		return domain.Topic{}, err
	}
	if len(topics) >= maxTopicsPerRoadmap {
		return domain.Topic{}, domain.ValidationError{Field: "topics", Reason: "roadmap topic limit reached"}
	}
	if err := ValidateHierarchy(topics, uuid.New(), in.ParentID); err != nil {
		return domain.Topic{}, err
	}
	kind, status := in.Kind, in.TaskStatus
	if kind == "" {
		kind = domain.ElementTopic
	}
	if status == "" {
		status = domain.TaskNotStarted
	}
	var t domain.Topic
	priority := in.Priority
	if priority == "" {
		priority = domain.PriorityImportant
	}
	if s, ok := a.store.(TopicPriorityCreateStore); ok {
		t, err = s.CreateTopicWithPriority(ctx, p.UserID, roadmap, in.ParentID, strings.TrimSpace(in.Title), in.Description, in.SortOrder, kind, status, priority)
	} else if s, ok := a.store.(TopicKindStore); ok {
		t, err = s.CreateTopicWithKind(ctx, p.UserID, roadmap, in.ParentID, strings.TrimSpace(in.Title), in.Description, in.SortOrder, kind, status)
	} else {
		t, err = a.store.CreateTopic(ctx, p.UserID, roadmap, in.ParentID, strings.TrimSpace(in.Title), in.Description, in.SortOrder)
	}
	if err == nil {
		_ = a.store.Audit(ctx, p.UserID, p.Actor, "create", "topic", &t.ID, map[string]any{"roadmap_id": roadmap})
	}
	return t, err
}

func (a *App) CreateTopicsBatch(ctx context.Context, p domain.Principal, roadmap uuid.UUID, inputs []domain.TopicInput) ([]domain.Topic, error) {
	if err := write(p); err != nil {
		return nil, err
	}
	if len(inputs) == 0 || len(inputs) > a.maxBatchTopics {
		return nil, domain.ValidationError{Field: "topics", Reason: fmt.Sprintf("batch must contain 1-%d topics", a.maxBatchTopics)}
	}
	existing, err := a.store.ListTopics(ctx, p.UserID, roadmap)
	if err != nil {
		return nil, err
	}
	if len(existing)+len(inputs) > maxTopicsPerRoadmap {
		return nil, domain.ValidationError{Field: "topics", Reason: "roadmap topic limit reached"}
	}
	refs := map[string]uuid.UUID{}
	items := make([]BatchTopic, len(inputs))
	all := append([]domain.Topic(nil), existing...)
	for i, in := range inputs {
		if err := validateTopic(in.Title, in.Description); err != nil {
			return nil, err
		}
		if err := validateTopicKind(in.Kind, in.TaskStatus); err != nil {
			return nil, err
		}
		if err := validatePriority(in.Priority); err != nil {
			return nil, err
		}
		id := uuid.New()
		if in.ClientRef != "" {
			if _, ok := refs[in.ClientRef]; ok {
				return nil, domain.ValidationError{Field: "topics", Reason: "duplicate client_ref"}
			}
			refs[in.ClientRef] = id
		}
		priority := in.Priority
		if priority == "" {
			priority = domain.PriorityImportant
		}
		items[i] = BatchTopic{
			ID:          id,
			Title:       strings.TrimSpace(in.Title),
			Description: in.Description,
			SortOrder:   in.SortOrder,
			Kind:        in.Kind,
			TaskStatus:  in.TaskStatus,
			Priority:    priority,
		}
	}
	for i, in := range inputs {
		parent := in.ParentID
		if in.ParentRef != "" {
			id, ok := refs[in.ParentRef]
			if !ok {
				return nil, domain.ValidationError{Field: "parent_ref", Reason: "unknown client_ref"}
			}
			parent = &id
		}
		items[i].ParentID = parent
		all = append(all, domain.Topic{ID: items[i].ID, ParentID: parent})
		if err := ValidateHierarchy(all, items[i].ID, parent); err != nil {
			return nil, err
		}
	}
	out, err := a.store.CreateTopicsBatch(ctx, p.UserID, roadmap, items)
	if err == nil {
		_ = a.store.Audit(ctx, p.UserID, p.Actor, "create_batch", "roadmap", &roadmap, map[string]any{"count": len(out)})
	}
	return out, err
}

type TopicPatch struct {
	ParentID    *uuid.UUID
	ParentSet   bool
	Title       *string
	Description *string
	SortOrder   *int
	Kind        *string
	TaskStatus  *string
	Status      *string
	Priority    *string
}

func (a *App) UpdateTopic(ctx context.Context, p domain.Principal, id uuid.UUID, in TopicPatch) (domain.Topic, error) {
	if err := write(p); err != nil {
		return domain.Topic{}, err
	}
	cur, err := a.store.GetTopic(ctx, p.UserID, id)
	if err != nil {
		return domain.Topic{}, err
	}
	parent := cur.ParentID
	if in.ParentSet {
		parent = in.ParentID
	}
	title := cur.Title
	if in.Title != nil {
		title = strings.TrimSpace(*in.Title)
	}
	desc := cur.Description
	if in.Description != nil {
		desc = *in.Description
	}
	order := cur.SortOrder
	if in.SortOrder != nil {
		order = *in.SortOrder
	}
	kind, status := cur.Kind, cur.TaskStatus
	if kind == "" {
		kind = domain.ElementTopic
	}
	if status == "" {
		status = domain.TaskNotStarted
	}
	if in.Kind != nil {
		kind = *in.Kind
	}
	if in.TaskStatus != nil {
		status = *in.TaskStatus
	}
	if in.Status != nil {
		if err := validateTopicStatus(*in.Status); err != nil {
			return domain.Topic{}, err
		}
		if kind == domain.ElementTask {
			return domain.Topic{}, domain.ValidationError{Field: "status", Reason: "task status is separate"}
		}
	}
	if in.Priority != nil {
		if err := validatePriority(*in.Priority); err != nil {
			return domain.Topic{}, err
		}
	}
	if err := validateTopicKind(kind, status); err != nil {
		return domain.Topic{}, err
	}
	if err := validateTopic(title, desc); err != nil {
		return domain.Topic{}, err
	}
	topics, err := a.store.ListTopics(ctx, p.UserID, cur.RoadmapID)
	if err != nil {
		return domain.Topic{}, err
	}
	if err := ValidateHierarchy(topics, id, parent); err != nil {
		return domain.Topic{}, err
	}
	var t domain.Topic
	if s, ok := a.store.(TopicKindStore); ok {
		t, err = s.UpdateTopicWithKind(ctx, p.UserID, id, parent, title, desc, order, kind, status)
	} else {
		t, err = a.store.UpdateTopic(ctx, p.UserID, id, parent, title, desc, order)
	}
	if err == nil {
		if m, ok := a.store.(TopicMetadataStore); ok {
			if in.Status != nil && kind == domain.ElementTopic {
				if err = m.SetTopicStatus(ctx, p.UserID, id, *in.Status); err != nil {
					return domain.Topic{}, err
				}
			}
			if in.Priority != nil {
				if err = m.SetTopicPriority(ctx, p.UserID, id, *in.Priority); err != nil {
					return domain.Topic{}, err
				}
			}
			if refreshed, e := a.store.GetTopic(ctx, p.UserID, id); e == nil {
				t = refreshed
			}
		}
		_ = a.store.Audit(ctx, p.UserID, p.Actor, "update", "topic", &id, nil)
	}
	return t, err
}

func (a *App) SetTopicStatus(ctx context.Context, p domain.Principal, id uuid.UUID, status string) error {
	if err := write(p); err != nil {
		return err
	}
	if err := validateTopicStatus(status); err != nil {
		return err
	}
	if status == "" {
		return domain.ValidationError{Field: "status", Reason: "is required"}
	}
	cur, err := a.store.GetTopic(ctx, p.UserID, id)
	if err == nil && cur.Kind == domain.ElementTask {
		return domain.ValidationError{Field: "status", Reason: "task status is separate"}
	}
	m, ok := a.store.(TopicMetadataStore)
	if !ok {
		return fmt.Errorf("topic status unavailable")
	}
	if err := m.SetTopicStatus(ctx, p.UserID, id, status); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "status", "topic", &id, map[string]any{"status": status})
}

func (a *App) SetTopicPriority(ctx context.Context, p domain.Principal, id uuid.UUID, priority string) error {
	if err := write(p); err != nil {
		return err
	}
	if err := validatePriority(priority); err != nil {
		return err
	}
	m, ok := a.store.(TopicMetadataStore)
	if !ok {
		return fmt.Errorf("topic priority unavailable")
	}
	if err := m.SetTopicPriority(ctx, p.UserID, id, priority); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "priority", "topic", &id, map[string]any{"priority": priority})
}

func (a *App) DeleteTopic(ctx context.Context, p domain.Principal, id uuid.UUID) error {
	if err := write(p); err != nil {
		return err
	}
	if err := a.store.DeleteTopic(ctx, p.UserID, id); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "delete", "topic", &id, nil)
}

func (a *App) SetCompletion(ctx context.Context, p domain.Principal, id uuid.UUID, completed bool) error {
	if err := write(p); err != nil {
		return err
	}
	cur, err := a.store.GetTopic(ctx, p.UserID, id)
	if err == nil && cur.Kind == domain.ElementTask {
		return domain.ValidationError{Field: "completed", Reason: "use task_status for tasks"}
	}
	if err := a.store.SetCompletion(ctx, p.UserID, id, completed); err != nil {
		return err
	}
	return a.store.Audit(ctx, p.UserID, p.Actor, "completion", "topic", &id, map[string]any{"completed": completed})
}

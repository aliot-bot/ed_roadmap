package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/repository"
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
	store           repository.Store
	sessionTTL      time.Duration
	maxBatchTopics  int
	accessKeyPepper string
	reportProvider  LearningReportProvider
}
type LearningReportProvider interface {
	Analyze(context.Context, domain.LearningOverview, []domain.LearningActivityDay) (string, error)
}

func (a *App) SetLearningReportProvider(p LearningReportProvider) { a.reportProvider = p }

func New(store repository.Store) *App {
	return NewWithLimits(store, 14*24*time.Hour, defaultMaxBatchTopics)
}
func NewWithLimits(store repository.Store, sessionTTL time.Duration, maxBatchTopics int) *App {
	if sessionTTL <= 0 {
		sessionTTL = 14 * 24 * time.Hour
	}
	if maxBatchTopics < 1 || maxBatchTopics > 1000 {
		maxBatchTopics = defaultMaxBatchTopics
	}
	return &App{store: store, sessionTTL: sessionTTL, maxBatchTopics: maxBatchTopics}
}
func (a *App) SetAccessKeyPepper(pepper string) { a.accessKeyPepper = pepper }

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
func validateLogin(login string) error { return validateText("login", login, 3, 254) }
func (a *App) Login(ctx context.Context, login, password string) (LoginResult, error) {
	u, hash, err := a.store.FindUserByLogin(ctx, strings.TrimSpace(login))
	if err != nil || !security.CheckPassword(hash, password) {
		return LoginResult{}, domain.ErrUnauthorized
	}
	return a.newSession(ctx, u)
}
func (a *App) LoginWithKey(ctx context.Context, key string) (LoginResult, error) {
	s, ok := a.store.(repository.AccessKeyStore)
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
	err = a.store.CreateSession(ctx, domain.Session{ID: uuid.New(), UserID: u.ID, TokenHash: security.TokenHash(token), CSRFHash: security.TokenHash(csrf), ExpiresAt: expires})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{User: u, SessionToken: token, CSRFToken: csrf, ExpiresAt: expires}, nil
}
func (a *App) AuthenticateSession(ctx context.Context, token string) (domain.Principal, []byte, error) {
	if token == "" {
		return domain.Principal{}, nil, domain.ErrUnauthorized
	}
	s, err := a.store.FindSession(ctx, security.TokenHash(token), time.Now().UTC())
	if err != nil {
		return domain.Principal{}, nil, domain.ErrUnauthorized
	}
	return domain.Principal{UserID: s.UserID, Scopes: map[string]bool{domain.ScopeRead: true, domain.ScopeWrite: true}, Actor: "browser"}, s.CSRFHash, nil
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
	if s, ok := a.store.(repository.TopicPriorityCreateStore); ok {
		t, err = s.CreateTopicWithPriority(ctx, p.UserID, roadmap, in.ParentID, strings.TrimSpace(in.Title), in.Description, in.SortOrder, kind, status, priority)
	} else if s, ok := a.store.(repository.TopicKindStore); ok {
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
	items := make([]repository.BatchTopic, len(inputs))
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
		items[i] = repository.BatchTopic{ID: id, Title: strings.TrimSpace(in.Title), Description: in.Description, SortOrder: in.SortOrder, Kind: in.Kind, TaskStatus: in.TaskStatus, Priority: priority}
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
	if s, ok := a.store.(repository.TopicKindStore); ok {
		t, err = s.UpdateTopicWithKind(ctx, p.UserID, id, parent, title, desc, order, kind, status)
	} else {
		t, err = a.store.UpdateTopic(ctx, p.UserID, id, parent, title, desc, order)
	}
	if err == nil {
		if m, ok := a.store.(repository.TopicMetadataStore); ok {
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
	m, ok := a.store.(repository.TopicMetadataStore)
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
	m, ok := a.store.(repository.TopicMetadataStore)
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

func (a *App) Analytics(ctx context.Context, p domain.Principal) (domain.Analytics, error) {
	roadmaps, err := a.ListRoadmaps(ctx, p)
	if err != nil {
		return domain.Analytics{}, err
	}
	out := domain.Analytics{RoadmapCount: len(roadmaps), Roadmaps: make([]domain.RoadmapAnalytics, 0, len(roadmaps))}
	for _, roadmap := range roadmaps {
		if roadmap.Progress == nil {
			continue
		}
		summary := summarizeProgress(*roadmap.Progress)
		out.Roadmaps = append(out.Roadmaps, domain.RoadmapAnalytics{ID: roadmap.ID, Title: roadmap.Title, LeafTopics: summary})
		out.LeafTopics.Total += summary.Total
		out.LeafTopics.Completed += summary.Completed
	}
	out.LeafTopics.Remaining = out.LeafTopics.Total - out.LeafTopics.Completed
	if out.LeafTopics.Total > 0 {
		percent := float64(out.LeafTopics.Completed) * 100 / float64(out.LeafTopics.Total)
		out.LeafTopics.Percent = &percent
	}
	return out, nil
}

func summarizeProgress(progress domain.Progress) domain.ProgressSummary {
	return domain.ProgressSummary{Total: progress.Total, Completed: progress.Completed, Remaining: progress.Total - progress.Completed, Percent: progress.Percent}
}

func (a *App) learningStore() (repository.LearningStore, error) {
	s, ok := a.store.(repository.LearningStore)
	if !ok {
		return nil, fmt.Errorf("learning sessions unavailable")
	}
	return s, nil
}
func (a *App) StartLearning(ctx context.Context, p domain.Principal, roadmap, element uuid.UUID) (domain.LearningSession, error) {
	if err := write(p); err != nil {
		return domain.LearningSession{}, err
	}
	s, err := a.learningStore()
	if err != nil {
		return domain.LearningSession{}, err
	}
	return s.StartLearningSession(ctx, p.UserID, roadmap, element, time.Now().UTC())
}
func (a *App) StartLearningElement(ctx context.Context, p domain.Principal, element uuid.UUID) (domain.LearningSession, error) {
	if err := read(p); err != nil {
		return domain.LearningSession{}, err
	}
	t, err := a.store.GetTopic(ctx, p.UserID, element)
	if err != nil {
		return domain.LearningSession{}, err
	}
	return a.StartLearning(ctx, p, t.RoadmapID, element)
}
func (a *App) ActiveLearning(ctx context.Context, p domain.Principal) (*domain.LearningSession, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	s, err := a.learningStore()
	if err != nil {
		return nil, err
	}
	// The filtered query is represented by a sentinel lookup in the postgres implementation.
	item, err := s.GetLearningSession(ctx, p.UserID, uuid.Nil)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
func (a *App) TransitionLearning(ctx context.Context, p domain.Principal, id uuid.UUID, status string, note *string) (domain.LearningSession, error) {
	if err := write(p); err != nil {
		return domain.LearningSession{}, err
	}
	s, err := a.learningStore()
	if err != nil {
		return domain.LearningSession{}, err
	}
	if status != domain.LearningPaused && status != domain.LearningActive && status != domain.LearningFinished {
		return domain.LearningSession{}, domain.ValidationError{Field: "status", Reason: "must be paused, active, or finished"}
	}
	return s.TransitionLearningSession(ctx, p.UserID, id, status, time.Now().UTC(), note)
}
func (a *App) LearningSessions(ctx context.Context, p domain.Principal, from, to time.Time) ([]domain.LearningSession, error) {
	if err := read(p); err != nil {
		return nil, err
	}
	s, err := a.learningStore()
	if err != nil {
		return nil, err
	}
	return s.ListLearningSessions(ctx, p.UserID, from, to)
}
func (a *App) LearningOverview(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) (domain.LearningOverview, error) {
	items, err := a.LearningSessions(ctx, p, from, to)
	if err != nil {
		return domain.LearningOverview{}, err
	}
	if len(items) == 0 {
		return domain.LearningOverview{}, nil
	}
	total, topic, task := 0, 0, 0
	days := map[string]bool{}
	for _, item := range items {
		mins := int(item.DurationSeconds / 60)
		total += mins
		if item.Kind == domain.ElementTask {
			task += mins
		} else {
			topic += mins
		}
		days[item.StartedAt.In(loc).Format("2006-01-02")] = true
	}
	return domain.LearningOverview{StudyMinutes: &total, TopicMinutes: &topic, TaskMinutes: &task, ActiveDays: ptrInt(len(days))}, nil
}
func (a *App) LearningActivity(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) ([]domain.LearningActivityDay, error) {
	items, err := a.LearningSessions(ctx, p, from, to)
	if err != nil {
		return nil, err
	}
	byDaySeconds := map[string]int64{}
	if segStore, ok := a.store.(repository.LearningSegmentsStore); ok {
		segments, e := segStore.ListLearningSegments(ctx, p.UserID, from, to)
		if e != nil {
			return nil, e
		}
		for _, seg := range segments {
			end := time.Now().UTC()
			if seg.EndedAt != nil {
				end = *seg.EndedAt
			}
			if end.Before(from) || !seg.StartedAt.Before(to) {
				continue
			}
			start := seg.StartedAt
			if start.Before(from) {
				start = from
			}
			if end.After(to) {
				end = to
			}
			for start.Before(end) {
				next := end
				localNext := start.In(loc).AddDate(0, 0, 1)
				boundary := time.Date(localNext.Year(), localNext.Month(), localNext.Day(), 0, 0, 0, 0, loc).In(time.UTC)
				if boundary.Before(next) {
					next = boundary
				}
				seconds := int64(next.Sub(start).Seconds())
				if seconds > 0 {
					byDaySeconds[start.In(loc).Format("2006-01-02")] += seconds
				}
				start = next
			}
		}
	} else {
		for _, item := range items {
			byDaySeconds[item.StartedAt.In(loc).Format("2006-01-02")] += item.DurationSeconds
		}
	}
	out := make([]domain.LearningActivityDay, 0, len(byDaySeconds))
	for date, seconds := range byDaySeconds {
		minutes := int(seconds / 60)
		level := "low"
		if minutes == 0 {
			level = "none"
		} else if minutes >= 120 {
			level = "max"
		} else if minutes >= 60 {
			level = "high"
		} else if minutes >= 30 {
			level = "moderate"
		}
		ach := []string{}
		if minutes >= 240 {
			ach = append(ach, "four_hours")
		}
		if minutes >= 360 {
			ach = append(ach, "six_hours")
		}
		if minutes >= 480 {
			ach = append(ach, "eight_hours")
		}
		out = append(out, domain.LearningActivityDay{Date: date, StudyMinutes: minutes, Intensity: level, Achievements: ach})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}
func (a *App) LearningAchievements(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) (domain.LearningAchievements, error) {
	days, err := a.LearningActivity(ctx, p, from, to, loc)
	if err != nil {
		return domain.LearningAchievements{}, err
	}
	out := domain.LearningAchievements{}
	for _, d := range days {
		for _, x := range d.Achievements {
			switch x {
			case "four_hours":
				out.FourHours++
			case "six_hours":
				out.SixHours++
			case "eight_hours":
				out.EightHours++
			}
		}
	}
	return out, nil
}

func activityIntensity(minutes int) (string, []string) {
	level := "low"
	if minutes == 0 {
		level = "none"
	} else if minutes >= 120 {
		level = "max"
	} else if minutes >= 60 {
		level = "high"
	} else if minutes >= 30 {
		level = "moderate"
	}
	ach := []string{}
	if minutes >= 240 {
		ach = append(ach, "four_hours")
	}
	if minutes >= 360 {
		ach = append(ach, "six_hours")
	}
	if minutes >= 480 {
		ach = append(ach, "eight_hours")
	}
	return level, ach
}
func (a *App) LatestLearningReport(ctx context.Context, p domain.Principal) (domain.LearningReport, error) {
	if err := read(p); err != nil {
		return domain.LearningReport{}, err
	}
	s, ok := a.store.(repository.ReportStore)
	if !ok {
		return domain.LearningReport{}, fmt.Errorf("reports unavailable")
	}
	return s.GetLatestLearningReport(ctx, p.UserID)
}
func (a *App) SaveImage(ctx context.Context, p domain.Principal, topic uuid.UUID, mime string, data []byte) (domain.UploadedImage, error) {
	if err := write(p); err != nil {
		return domain.UploadedImage{}, err
	}
	if len(data) == 0 || len(data) > 10*1024*1024 {
		return domain.UploadedImage{}, domain.ValidationError{Field: "file", Reason: "must be 1-10 MB"}
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width > 10000 || config.Height > 10000 {
		return domain.UploadedImage{}, domain.ValidationError{Field: "file", Reason: "invalid or oversized image"}
	}
	detected := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if detected == "" || mime != detected {
		return domain.UploadedImage{}, domain.ValidationError{Field: "mime_type", Reason: "unsupported image type"}
	}
	s, ok := a.store.(repository.ImageStore)
	if !ok {
		return domain.UploadedImage{}, fmt.Errorf("image storage unavailable")
	}
	return s.SaveImage(ctx, p.UserID, topic, mime, data)
}
func (a *App) GetImage(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.UploadedImage, error) {
	if err := read(p); err != nil {
		return domain.UploadedImage{}, err
	}
	s, ok := a.store.(repository.ImageStore)
	if !ok {
		return domain.UploadedImage{}, fmt.Errorf("image storage unavailable")
	}
	return s.GetImage(ctx, p.UserID, id)
}
func (a *App) AnalyzeLearning(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) (domain.LearningReport, error) {
	if err := write(p); err != nil {
		return domain.LearningReport{}, err
	}
	s, ok := a.store.(repository.ReportStore)
	if !ok {
		return domain.LearningReport{}, fmt.Errorf("reports unavailable")
	}
	overview, err := a.LearningOverview(ctx, p, from, to, loc)
	if err != nil {
		return domain.LearningReport{}, err
	}
	activity, err := a.LearningActivity(ctx, p, from, to, loc)
	if err != nil {
		return domain.LearningReport{}, err
	}
	report := domain.LearningReport{Status: "unavailable", Reason: "AI provider is not configured"}
	if a.reportProvider != nil {
		md, e := a.reportProvider.Analyze(ctx, overview, activity)
		if e != nil {
			report.Status = "failed"
			report.Reason = "AI provider failed"
		} else {
			report.Status = "ready"
			report.Markdown = md
		}
	}
	return s.SaveLearningReport(ctx, p.UserID, report)
}
func ptrInt(v int) *int { return &v }
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

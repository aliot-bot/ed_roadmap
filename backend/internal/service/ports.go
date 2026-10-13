package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

// AuthStore is the persistence boundary for browser and MCP authentication.
type AuthStore interface {
	BootstrapUser(context.Context, string, string) (domain.User, error)
	FindUserByLogin(context.Context, string) (domain.User, string, error)
	FindUser(context.Context, uuid.UUID) (domain.User, error)
	CreateSession(context.Context, domain.Session) error
	FindSession(context.Context, []byte, time.Time) (domain.Session, error)
	DeleteSession(context.Context, []byte) error
}

type RoadmapStore interface {
	ListRoadmaps(context.Context, uuid.UUID) ([]domain.Roadmap, error)
	GetRoadmap(context.Context, uuid.UUID, uuid.UUID) (domain.Roadmap, error)
	CreateRoadmap(context.Context, uuid.UUID, string, string) (domain.Roadmap, error)
	UpdateRoadmap(context.Context, uuid.UUID, uuid.UUID, *string, *string) (domain.Roadmap, error)
	DeleteRoadmap(context.Context, uuid.UUID, uuid.UUID) error
	GetRoadmapCover(context.Context, uuid.UUID, uuid.UUID) (domain.RoadmapCover, error)
	SetRoadmapCover(context.Context, uuid.UUID, uuid.UUID, string, []byte) (domain.RoadmapCover, error)
	DeleteRoadmapCover(context.Context, uuid.UUID, uuid.UUID) error
}

type TopicStore interface {
	ListTopics(context.Context, uuid.UUID, uuid.UUID) ([]domain.Topic, error)
	GetTopic(context.Context, uuid.UUID, uuid.UUID) (domain.Topic, error)
	CreateTopic(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int) (domain.Topic, error)
	CreateTopicsBatch(context.Context, uuid.UUID, uuid.UUID, []BatchTopic) ([]domain.Topic, error)
	UpdateTopic(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int) (domain.Topic, error)
	DeleteTopic(context.Context, uuid.UUID, uuid.UUID) error
	SetCompletion(context.Context, uuid.UUID, uuid.UUID, bool) error
}

type ContentStore interface {
	GetNote(context.Context, uuid.UUID, uuid.UUID) (domain.Note, error)
	SetNote(context.Context, uuid.UUID, uuid.UUID, string) error
	ListChecklist(context.Context, uuid.UUID, uuid.UUID) ([]domain.ChecklistItem, error)
	SetChecklist(context.Context, uuid.UUID, uuid.UUID, []domain.ChecklistItemInput) ([]domain.ChecklistItem, error)
	AppendChecklist(context.Context, uuid.UUID, uuid.UUID, []string) ([]domain.ChecklistItem, error)
	GetTopicDocument(context.Context, uuid.UUID, uuid.UUID) (domain.TopicDocument, error)
	SetTopicDocument(context.Context, uuid.UUID, uuid.UUID, string) (domain.TopicDocument, error)
}

type GraphStore interface {
	ListDependencies(context.Context, uuid.UUID, uuid.UUID) ([]domain.Dependency, error)
	ReplaceDependencies(context.Context, uuid.UUID, uuid.UUID, []domain.Dependency) error
}

type LayoutStore interface {
	ListLayouts(context.Context, uuid.UUID, uuid.UUID) ([]domain.Layout, error)
	ReplaceLayouts(context.Context, uuid.UUID, uuid.UUID, []domain.Layout) error
}

type AuditStore interface {
	Audit(context.Context, uuid.UUID, string, string, string, *uuid.UUID, map[string]any) error
}

type IdempotencyStore interface {
	GetIdempotency(context.Context, uuid.UUID, string, []byte) ([]byte, bool, error)
	PutIdempotency(context.Context, uuid.UUID, string, []byte, []byte) error
}

// Store is the composite persistence boundary used directly by App. Its
// cohesive embedded ports keep individual use cases dependent on smaller
// contracts while retaining one constructor boundary for the application.
type Store interface {
	AuthStore
	RoadmapStore
	TopicStore
	ContentStore
	GraphStore
	LayoutStore
	AuditStore
	IdempotencyStore
}

// AccessKeyStore is optional because the main application does not need
// access-key persistence for ordinary session authentication.
type AccessKeyStore interface {
	FindUserByAccessKey(context.Context, []byte) (domain.User, error)
}

// TopicKindStore is optional for repositories that persist task metadata.
type TopicKindStore interface {
	CreateTopicWithKind(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int, string, string) (domain.Topic, error)
	UpdateTopicWithKind(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int, string, string) (domain.Topic, error)
}

// TopicMetadataStore is optional for adapters that support status and
// priority updates independently of the structural topic fields.
type TopicMetadataStore interface {
	SetTopicStatus(context.Context, uuid.UUID, uuid.UUID, string) error
	SetTopicPriority(context.Context, uuid.UUID, uuid.UUID, string) error
}

type TopicPriorityCreateStore interface {
	CreateTopicWithPriority(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, string, string, int, string, string, string) (domain.Topic, error)
}

type LearningStore interface {
	StartLearningSession(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) (domain.LearningSession, error)
	GetLearningSession(context.Context, uuid.UUID, uuid.UUID) (domain.LearningSession, error)
	TransitionLearningSession(context.Context, uuid.UUID, uuid.UUID, string, time.Time, *string) (domain.LearningSession, error)
	ListLearningSessions(context.Context, uuid.UUID, time.Time, time.Time) ([]domain.LearningSession, error)
}

type LearningSegmentsStore interface {
	ListLearningSegments(context.Context, uuid.UUID, time.Time, time.Time) ([]LearningSegment, error)
}

type ReportStore interface {
	GetLatestLearningReport(context.Context, uuid.UUID) (domain.LearningReport, error)
	SaveLearningReport(context.Context, uuid.UUID, domain.LearningReport) (domain.LearningReport, error)
}

type ImageStore interface {
	SaveImage(context.Context, uuid.UUID, uuid.UUID, string, []byte) (domain.UploadedImage, error)
	GetImage(context.Context, uuid.UUID, uuid.UUID) (domain.UploadedImage, error)
}

type BatchTopic struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Title       string
	Description string
	SortOrder   int
	Kind        string
	TaskStatus  string
	Priority    string
}

type LearningSegment struct {
	ID        uuid.UUID
	SessionID uuid.UUID
	StartedAt time.Time
	EndedAt   *time.Time
}

package httpapi

import (
	"context"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
	"roadmap/backend/internal/service"
)

// LoginResult and TopicPatch remain service-owned DTOs; the aliases let the
// transport declare its own consumer-facing ports without accepting App.
type LoginResult = service.LoginResult
type TopicPatch = service.TopicPatch

type AuthApplication interface {
	Bootstrap(context.Context, string, string) (LoginResult, error)
	Login(context.Context, string, string) (LoginResult, error)
	LoginWithKey(context.Context, string) (LoginResult, error)
	AuthenticateSession(context.Context, string) (domain.Principal, []byte, error)
	Logout(context.Context, string) error
	User(context.Context, domain.Principal) (domain.User, error)
	MCPPrincipal(context.Context, string, []string) (domain.Principal, error)
}

type RoadmapApplication interface {
	ListRoadmaps(context.Context, domain.Principal) ([]domain.Roadmap, error)
	GetRoadmap(context.Context, domain.Principal, uuid.UUID) (domain.Roadmap, error)
	CreateRoadmap(context.Context, domain.Principal, string, string) (domain.Roadmap, error)
	UpdateRoadmap(context.Context, domain.Principal, uuid.UUID, *string, *string) (domain.Roadmap, error)
	DeleteRoadmap(context.Context, domain.Principal, uuid.UUID) error
	GetRoadmapCover(context.Context, domain.Principal, uuid.UUID) (domain.RoadmapCover, error)
	SetRoadmapCover(context.Context, domain.Principal, uuid.UUID, string, []byte) (domain.RoadmapCover, error)
	DeleteRoadmapCover(context.Context, domain.Principal, uuid.UUID) error
}

type TopicApplication interface {
	ListTopics(context.Context, domain.Principal, uuid.UUID) ([]domain.Topic, error)
	CreateTopic(context.Context, domain.Principal, uuid.UUID, domain.TopicInput) (domain.Topic, error)
	CreateTopicsBatch(context.Context, domain.Principal, uuid.UUID, []domain.TopicInput) ([]domain.Topic, error)
	UpdateTopic(context.Context, domain.Principal, uuid.UUID, TopicPatch) (domain.Topic, error)
	DeleteTopic(context.Context, domain.Principal, uuid.UUID) error
	SetCompletion(context.Context, domain.Principal, uuid.UUID, bool) error
	SetTopicStatus(context.Context, domain.Principal, uuid.UUID, string) error
	SetTopicPriority(context.Context, domain.Principal, uuid.UUID, string) error
}

type ContentApplication interface {
	GetNote(context.Context, domain.Principal, uuid.UUID) (domain.Note, error)
	SetNote(context.Context, domain.Principal, uuid.UUID, string) error
	Checklist(context.Context, domain.Principal, uuid.UUID) ([]domain.ChecklistItem, error)
	SetChecklist(context.Context, domain.Principal, uuid.UUID, []domain.ChecklistItemInput) ([]domain.ChecklistItem, error)
	AppendChecklist(context.Context, domain.Principal, uuid.UUID, []string) ([]domain.ChecklistItem, error)
	GetDocument(context.Context, domain.Principal, uuid.UUID) (domain.TopicDocument, error)
	SetDocument(context.Context, domain.Principal, uuid.UUID, string) (domain.TopicDocument, error)
}

type GraphApplication interface {
	Dependencies(context.Context, domain.Principal, uuid.UUID) ([]domain.Dependency, error)
	SetDependencies(context.Context, domain.Principal, uuid.UUID, []domain.Dependency) error
	Progress(context.Context, domain.Principal, uuid.UUID) (domain.Progress, error)
}

type LearningApplication interface {
	Analytics(context.Context, domain.Principal) (domain.Analytics, error)
	LearningOverview(context.Context, domain.Principal, time.Time, time.Time, *time.Location) (domain.LearningOverview, error)
	LearningActivity(context.Context, domain.Principal, time.Time, time.Time, *time.Location) ([]domain.LearningActivityDay, error)
	LearningAchievements(context.Context, domain.Principal, time.Time, time.Time, *time.Location) (domain.LearningAchievements, error)
	LatestLearningReport(context.Context, domain.Principal) (domain.LearningReport, error)
	AnalyzeLearning(context.Context, domain.Principal, time.Time, time.Time, *time.Location) (domain.LearningReport, error)
	ActiveLearning(context.Context, domain.Principal) (*domain.LearningSession, error)
	LearningSessions(context.Context, domain.Principal, time.Time, time.Time) ([]domain.LearningSession, error)
	StartLearningElement(context.Context, domain.Principal, uuid.UUID) (domain.LearningSession, error)
	TransitionLearning(context.Context, domain.Principal, uuid.UUID, string, *string) (domain.LearningSession, error)
	SaveImage(context.Context, domain.Principal, uuid.UUID, string, []byte) (domain.UploadedImage, error)
	GetImage(context.Context, domain.Principal, uuid.UUID) (domain.UploadedImage, error)
}

type LayoutApplication interface {
	Layouts(context.Context, domain.Principal, uuid.UUID) ([]domain.Layout, error)
	SetLayouts(context.Context, domain.Principal, uuid.UUID, []domain.Layout) error
}

type MCPApplication interface {
	Idempotent(context.Context, domain.Principal, string, any, func() (any, error)) (any, error)
}

type Application interface {
	AuthApplication
	RoadmapApplication
	TopicApplication
	ContentApplication
	GraphApplication
	LearningApplication
	LayoutApplication
	MCPApplication
}

type Logger interface {
	Warn(string, ...any)
}

type accessKeyPepperConfigurator interface {
	SetAccessKeyPepper(string)
}

var (
	_ Application                 = (*service.App)(nil)
	_ accessKeyPepperConfigurator = (*service.App)(nil)
)

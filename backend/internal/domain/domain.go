package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation failed")
)

type User struct {
	ID          uuid.UUID `json:"id"`
	Login       string    `json:"login"`
	DisplayName string    `json:"display_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
type LearningReport struct {
	ID        uuid.UUID `json:"id"`
	Markdown  string    `json:"markdown"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
}
type UploadedImage struct {
	ID        uuid.UUID `json:"id"`
	TopicID   uuid.UUID `json:"topic_id"`
	MIMEType  string    `json:"mime_type"`
	Size      int       `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Data      []byte    `json:"-"`
}
type Roadmap struct {
	ID          uuid.UUID `json:"id"`
	OwnerID     uuid.UUID `json:"-"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	HasCover    bool      `json:"has_cover,omitempty"`
	Progress    *Progress `json:"progress,omitempty"`
}

type RoadmapCover struct {
	MIMEType  string    `json:"-"`
	Data      []byte    `json:"-"`
	UpdatedAt time.Time `json:"-"`
}
type Topic struct {
	ID          uuid.UUID  `json:"id"`
	RoadmapID   uuid.UUID  `json:"roadmap_id"`
	ParentID    *uuid.UUID `json:"parent_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	SortOrder   int        `json:"sort_order"`
	Completed   bool       `json:"completed"`
	Kind        string     `json:"kind"`
	TaskStatus  string     `json:"task_status,omitempty"`
	Status      string     `json:"status,omitempty"`
	Priority    string     `json:"priority,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
type Dependency struct {
	PredecessorTopicID uuid.UUID `json:"predecessor_topic_id"`
	SuccessorTopicID   uuid.UUID `json:"successor_topic_id"`
}
type Layout struct {
	TopicID   uuid.UUID `json:"topic_id"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	Collapsed bool      `json:"collapsed"`
}
type Progress struct {
	Total        int             `json:"total"`
	Completed    int             `json:"completed"`
	Percent      *float64        `json:"percent"`
	Groups       []GroupProgress `json:"groups"`
	StatusCounts map[string]int  `json:"status_counts,omitempty"`
}
type GroupProgress struct {
	TopicID   uuid.UUID `json:"topic_id"`
	Total     int       `json:"total"`
	Completed int       `json:"completed"`
	Percent   *float64  `json:"percent"`
}
type RoadmapDetail struct {
	Roadmap      Roadmap      `json:"roadmap"`
	Topics       []Topic      `json:"topics"`
	Dependencies []Dependency `json:"dependencies"`
	Progress     Progress     `json:"progress"`
}
type TopicInput struct {
	ClientRef   string     `json:"client_ref,omitempty"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	ParentRef   string     `json:"parent_ref,omitempty"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	SortOrder   int        `json:"sort_order"`
	Kind        string     `json:"kind,omitempty"`
	TaskStatus  string     `json:"task_status,omitempty"`
	Status      string     `json:"status,omitempty"`
	Priority    string     `json:"priority,omitempty"`
}

type Principal struct {
	UserID uuid.UUID
	Scopes map[string]bool
	Actor  string
}

func (p Principal) Can(scope string) bool { return p.Scopes[scope] }

// ScopeRead and ScopeWrite are deliberately small. Browser sessions receive both;
// MCP bearer tokens can be configured with only ScopeRead.
const (
	ScopeRead         = "roadmaps:read"
	ScopeWrite        = "roadmaps:write"
	ElementTopic      = "topic"
	ElementTask       = "task"
	TaskNotStarted    = "not_started"
	TaskInProgress    = "in_progress"
	TaskCompleted     = "completed"
	TopicNotStarted   = "not_started"
	TopicInProgress   = "in_progress"
	TopicNeedsReview  = "needs_review"
	TopicCompleted    = "completed"
	PriorityCritical  = "critical"
	PriorityImportant = "important"
	PriorityOptional  = "optional"
	PrioritySkippable = "skippable"
	LearningActive    = "active"
	LearningPaused    = "paused"
	LearningFinished  = "finished"
)

type LearningSession struct {
	ID              uuid.UUID  `json:"id"`
	RoadmapID       uuid.UUID  `json:"roadmap_id"`
	ElementID       uuid.UUID  `json:"element_id"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	DurationSeconds int64      `json:"duration_seconds"`
	Note            string     `json:"note,omitempty"`
}
type LearningOverview struct {
	StudyMinutes *int `json:"study_minutes"`
	TopicMinutes *int `json:"topic_minutes"`
	TaskMinutes  *int `json:"task_minutes"`
	ActiveDays   *int `json:"active_days"`
	StreakDays   *int `json:"streak_days"`
}
type LearningActivityDay struct {
	Date            string   `json:"date"`
	StudyMinutes    int      `json:"study_minutes"`
	CompletedTopics *int     `json:"completed_topics"`
	CompletedTasks  *int     `json:"completed_tasks"`
	Accuracy        *float64 `json:"accuracy"`
	Intensity       string   `json:"intensity,omitempty"`
	Achievements    []string `json:"achievements,omitempty"`
}

type LearningAchievements struct {
	FourHours  int `json:"four_hours"`
	SixHours   int `json:"six_hours"`
	EightHours int `json:"eight_hours"`
}

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	CSRFHash  []byte
	ExpiresAt time.Time
}

type Note struct {
	Content string `json:"content"`
}

type ChecklistItem struct {
	ID        uuid.UUID `json:"id"`
	TopicID   uuid.UUID `json:"topic_id"`
	Text      string    `json:"text"`
	Completed bool      `json:"completed"`
	SortOrder int       `json:"sort_order"`
}

type ChecklistItemInput struct {
	ID        *uuid.UUID `json:"id,omitempty"`
	Text      string     `json:"text"`
	Completed bool       `json:"completed"`
	SortOrder int        `json:"sort_order"`
}

// TopicDocument is the long-form Markdown companion to a topic's short Note.
// Timestamps are nil until the first document is saved.
type TopicDocument struct {
	Content   string     `json:"content"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

type ProgressSummary struct {
	Total     int      `json:"total"`
	Completed int      `json:"completed"`
	Remaining int      `json:"remaining"`
	Percent   *float64 `json:"percent"`
}

type RoadmapAnalytics struct {
	ID         uuid.UUID       `json:"id"`
	Title      string          `json:"title"`
	LeafTopics ProgressSummary `json:"leaf_topics"`
}

type Analytics struct {
	RoadmapCount int                `json:"roadmap_count"`
	LeafTopics   ProgressSummary    `json:"leaf_topics"`
	Roadmaps     []RoadmapAnalytics `json:"roadmaps"`
}

type ValidationError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (e ValidationError) Error() string { return e.Field + ": " + e.Reason }
func (e ValidationError) Unwrap() error { return ErrValidation }

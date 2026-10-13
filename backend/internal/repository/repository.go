package repository

import "roadmap/backend/internal/service"

// Store remains a source-compatible alias while the canonical persistence
// port lives next to its service consumer.
type Store = service.Store

// These aliases keep repository-facing adapters and tests source-compatible
// while the boundary structs live next to their service consumers.
type BatchTopic = service.BatchTopic
type LearningSegment = service.LearningSegment

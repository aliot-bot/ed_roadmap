package repository

import "roadmap/backend/internal/service"

var (
	_ service.Store                    = (*Postgres)(nil)
	_ service.AccessKeyStore           = (*Postgres)(nil)
	_ service.TopicKindStore           = (*Postgres)(nil)
	_ service.TopicMetadataStore       = (*Postgres)(nil)
	_ service.TopicPriorityCreateStore = (*Postgres)(nil)
	_ service.LearningStore            = (*Postgres)(nil)
	_ service.LearningSegmentsStore    = (*Postgres)(nil)
	_ service.ReportStore              = (*Postgres)(nil)
	_ service.ImageStore               = (*Postgres)(nil)
)

package service

import (
	"context"

	"roadmap/backend/internal/domain"
)

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

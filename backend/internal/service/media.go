package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (a *App) LatestLearningReport(ctx context.Context, p domain.Principal) (domain.LearningReport, error) {
	if err := read(p); err != nil {
		return domain.LearningReport{}, err
	}
	s, ok := a.store.(ReportStore)
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
	s, ok := a.store.(ImageStore)
	if !ok {
		return domain.UploadedImage{}, fmt.Errorf("image storage unavailable")
	}
	return s.SaveImage(ctx, p.UserID, topic, mime, data)
}

func (a *App) GetImage(ctx context.Context, p domain.Principal, id uuid.UUID) (domain.UploadedImage, error) {
	if err := read(p); err != nil {
		return domain.UploadedImage{}, err
	}
	s, ok := a.store.(ImageStore)
	if !ok {
		return domain.UploadedImage{}, fmt.Errorf("image storage unavailable")
	}
	return s.GetImage(ctx, p.UserID, id)
}

func (a *App) AnalyzeLearning(ctx context.Context, p domain.Principal, from, to time.Time, loc *time.Location) (domain.LearningReport, error) {
	if err := write(p); err != nil {
		return domain.LearningReport{}, err
	}
	s, ok := a.store.(ReportStore)
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

func ptrInt(v int) *int {
	return &v
}

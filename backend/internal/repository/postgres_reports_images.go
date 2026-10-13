package repository

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) GetLatestLearningReport(ctx context.Context, user uuid.UUID) (domain.LearningReport, error) {
	var r domain.LearningReport
	err := p.db.QueryRow(ctx, `
		SELECT
			id,
			markdown,
			created_at,
			status,
			reason
		FROM learning_analysis_reports
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, user).Scan(&r.ID, &r.Markdown, &r.CreatedAt, &r.Status, &r.Reason)
	return r, notFound(err)
}

func (p *Postgres) SaveLearningReport(ctx context.Context, user uuid.UUID, r domain.LearningReport) (domain.LearningReport, error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	err := p.db.QueryRow(ctx, `
		INSERT INTO learning_analysis_reports (
			id,
			user_id,
			markdown,
			status,
			reason
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5
		)
		RETURNING
			created_at
	`, r.ID, user, r.Markdown, r.Status, r.Reason).Scan(&r.CreatedAt)
	return r, err
}

func (p *Postgres) SaveImage(ctx context.Context, user, topic uuid.UUID, mime string, data []byte) (domain.UploadedImage, error) {
	var x domain.UploadedImage
	err := p.db.QueryRow(ctx, `
		INSERT INTO uploaded_images (
			user_id,
			topic_id,
			mime_type,
			data
		)
		SELECT
			$1,
			t.id,
			$3,
			$4
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		WHERE t.id = $2
		  AND r.owner_id = $1
		RETURNING
			id,
			topic_id,
			mime_type,
			length(data),
			created_at
	`, user, topic, mime, data).Scan(&x.ID, &x.TopicID, &x.MIMEType, &x.Size, &x.CreatedAt)
	return x, notFound(err)
}

func (p *Postgres) GetImage(ctx context.Context, user, id uuid.UUID) (domain.UploadedImage, error) {
	var x domain.UploadedImage
	err := p.db.QueryRow(ctx, `
		SELECT
			i.id,
			i.topic_id,
			i.mime_type,
			length(i.data),
			i.created_at,
			i.data
		FROM uploaded_images i
		WHERE i.id = $1
		  AND i.user_id = $2
	`, id, user).Scan(&x.ID, &x.TopicID, &x.MIMEType, &x.Size, &x.CreatedAt, &x.Data)
	return x, notFound(err)
}

package repository

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) ListRoadmaps(ctx context.Context, owner uuid.UUID) ([]domain.Roadmap, error) {
	rows, err := p.db.Query(ctx, `
		SELECT
			r.id,
			r.owner_id,
			r.title,
			r.description,
			r.created_at,
			r.updated_at,
			EXISTS (
				SELECT 1
				FROM roadmap_covers c
				WHERE c.roadmap_id = r.id
			)
		FROM roadmaps r
		WHERE r.owner_id = $1
		ORDER BY
			r.updated_at DESC,
			r.id
	`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Roadmap{}
	for rows.Next() {
		r, e := scanRoadmap(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) GetRoadmap(ctx context.Context, owner, id uuid.UUID) (domain.Roadmap, error) {
	return scanRoadmap(p.db.QueryRow(ctx, `
		SELECT
			r.id,
			r.owner_id,
			r.title,
			r.description,
			r.created_at,
			r.updated_at,
			EXISTS (
				SELECT 1
				FROM roadmap_covers c
				WHERE c.roadmap_id = r.id
			)
		FROM roadmaps r
		WHERE r.id = $1
		  AND r.owner_id = $2
	`, id, owner))
}

func (p *Postgres) CreateRoadmap(ctx context.Context, owner uuid.UUID, title, desc string) (domain.Roadmap, error) {
	return scanRoadmap(p.db.QueryRow(ctx, `
		INSERT INTO roadmaps (
			owner_id,
			title,
			description
		)
		VALUES (
			$1,
			$2,
			$3
		)
		RETURNING
			id,
			owner_id,
			title,
			description,
			created_at,
			updated_at,
			false
	`, owner, title, desc))
}

func (p *Postgres) UpdateRoadmap(ctx context.Context, owner, id uuid.UUID, title, desc *string) (domain.Roadmap, error) {
	return scanRoadmap(p.db.QueryRow(ctx, `
		UPDATE roadmaps
		SET
			title = COALESCE($3, title),
			description = COALESCE($4, description),
			updated_at = now()
		WHERE id = $1
		  AND owner_id = $2
		RETURNING
			id,
			owner_id,
			title,
			description,
			created_at,
			updated_at,
			EXISTS (
				SELECT 1
				FROM roadmap_covers c
				WHERE c.roadmap_id = roadmaps.id
			)
	`, id, owner, title, desc))
}

func (p *Postgres) DeleteRoadmap(ctx context.Context, owner, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `
		DELETE FROM roadmaps
		WHERE id = $1
		  AND owner_id = $2
	`, id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (p *Postgres) GetRoadmapCover(ctx context.Context, owner, id uuid.UUID) (domain.RoadmapCover, error) {
	var cover domain.RoadmapCover
	err := p.db.QueryRow(ctx, `
		SELECT
			c.mime_type,
			c.data,
			c.updated_at
		FROM roadmap_covers c
		JOIN roadmaps r
			ON r.id = c.roadmap_id
		WHERE c.roadmap_id = $1
		  AND r.owner_id = $2
	`, id, owner).Scan(&cover.MIMEType, &cover.Data, &cover.UpdatedAt)
	return cover, notFound(err)
}

func (p *Postgres) SetRoadmapCover(ctx context.Context, owner, id uuid.UUID, mime string, data []byte) (domain.RoadmapCover, error) {
	var cover domain.RoadmapCover
	err := p.db.QueryRow(ctx, `
		INSERT INTO roadmap_covers (
			roadmap_id,
			mime_type,
			data
		)
		SELECT
			r.id,
			$3,
			$4
		FROM roadmaps r
		WHERE r.id = $1
		  AND r.owner_id = $2
		ON CONFLICT (roadmap_id) DO UPDATE
		SET
			mime_type = EXCLUDED.mime_type,
			data = EXCLUDED.data,
			updated_at = now()
		RETURNING
			mime_type,
			data,
			updated_at
	`, id, owner, mime, data).Scan(&cover.MIMEType, &cover.Data, &cover.UpdatedAt)
	return cover, notFound(err)
}

func (p *Postgres) DeleteRoadmapCover(ctx context.Context, owner, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `
		DELETE FROM roadmap_covers c
		USING roadmaps r
		WHERE c.roadmap_id = $1
		  AND r.id = c.roadmap_id
		  AND r.owner_id = $2
	`, id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

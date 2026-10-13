package repository

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) ListDependencies(ctx context.Context, owner, roadmap uuid.UUID) ([]domain.Dependency, error) {
	if _, err := p.GetRoadmap(ctx, owner, roadmap); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `
		SELECT
			predecessor_topic_id,
			successor_topic_id
		FROM topic_dependencies
		WHERE roadmap_id = $1
		ORDER BY
			predecessor_topic_id,
			successor_topic_id
	`, roadmap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Dependency{}
	for rows.Next() {
		var d domain.Dependency
		if e := rows.Scan(&d.PredecessorTopicID, &d.SuccessorTopicID); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceDependencies(ctx context.Context, owner, roadmap uuid.UUID, deps []domain.Dependency) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var ok bool
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM roadmaps
			WHERE id = $1
			  AND owner_id = $2
			FOR UPDATE
		)
	`, roadmap, owner).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}

	if _, err = tx.Exec(ctx, `
		DELETE FROM topic_dependencies
		WHERE roadmap_id = $1
	`, roadmap); err != nil {
		return err
	}

	for _, d := range deps {
		if _, err = tx.Exec(ctx, `
			INSERT INTO topic_dependencies (
				roadmap_id,
				predecessor_topic_id,
				successor_topic_id
			)
			VALUES (
				$1,
				$2,
				$3
			)
		`, roadmap, d.PredecessorTopicID, d.SuccessorTopicID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) ListLayouts(ctx context.Context, owner, roadmap uuid.UUID) ([]domain.Layout, error) {
	if _, err := p.GetRoadmap(ctx, owner, roadmap); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `
		SELECT
			topic_id,
			x,
			y,
			collapsed
		FROM topic_layouts
		WHERE roadmap_id = $1
		ORDER BY topic_id
	`, roadmap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Layout{}
	for rows.Next() {
		var l domain.Layout
		if e := rows.Scan(&l.TopicID, &l.X, &l.Y, &l.Collapsed); e != nil {
			return nil, e
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceLayouts(ctx context.Context, owner, roadmap uuid.UUID, items []domain.Layout) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var ok bool
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM roadmaps
			WHERE id = $1
			  AND owner_id = $2
			FOR UPDATE
		)
	`, roadmap, owner).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}

	for _, l := range items {
		tag, e := tx.Exec(ctx, `
			INSERT INTO topic_layouts (
				roadmap_id,
				topic_id,
				x,
				y,
				collapsed,
				updated_at
			)
			SELECT
				$1,
				t.id,
				$3,
				$4,
				$5,
				now()
			FROM topics t
			WHERE t.id = $2
			  AND t.roadmap_id = $1
			ON CONFLICT (roadmap_id, topic_id) DO UPDATE
			SET
				x = EXCLUDED.x,
				y = EXCLUDED.y,
				collapsed = EXCLUDED.collapsed,
				updated_at = now()
		`, roadmap, l.TopicID, l.X, l.Y, l.Collapsed)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrValidation
		}
	}
	return tx.Commit(ctx)
}

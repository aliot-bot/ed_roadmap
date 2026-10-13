package repository

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) SetCompletion(ctx context.Context, owner, id uuid.UUID, completed bool) error {
	var exists, saved bool
	err := p.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT
				t.id,
				NOT EXISTS (
					SELECT 1
					FROM topics child
					WHERE child.parent_id = t.id
				) AS is_leaf
			FROM topics t
			JOIN roadmaps r
				ON r.id = t.roadmap_id
			WHERE t.id = $1
			  AND r.owner_id = $2
			FOR UPDATE OF t
		), saved AS (
			INSERT INTO topic_progress (
				user_id,
				topic_id,
				completed,
				status,
				completed_at,
				updated_at
			)
			SELECT
				$2,
				id,
				$3,
				CASE
					WHEN $3 THEN 'completed'
					ELSE 'not_started'
				END,
				CASE
					WHEN $3 THEN now()
					ELSE NULL
				END,
				now()
			FROM candidate
			WHERE is_leaf
			ON CONFLICT (user_id, topic_id) DO UPDATE
			SET
				completed = EXCLUDED.completed,
				status = EXCLUDED.status,
				completed_at = EXCLUDED.completed_at,
				updated_at = now()
			RETURNING topic_id
		)
		SELECT
			EXISTS (
				SELECT 1
				FROM candidate
			),
			EXISTS (
				SELECT 1
				FROM saved
			)
	`, id, owner, completed).Scan(&exists, &saved)
	if err != nil {
		return err
	}

	if !exists {
		return domain.ErrNotFound
	}
	if !saved {
		return domain.ErrConflict
	}
	return nil
}

func (p *Postgres) SetTopicStatus(ctx context.Context, owner, id uuid.UUID, status string) error {
	completed := status == domain.TopicCompleted
	tag, err := p.db.Exec(ctx, `
		INSERT INTO topic_progress (
			user_id,
			topic_id,
			completed,
			status,
			completed_at,
			updated_at
		)
		SELECT
			$2,
			t.id,
			$3,
			$4,
			CASE
				WHEN $3 THEN now()
				ELSE NULL
			END,
			now()
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		WHERE t.id = $1
		  AND r.owner_id = $2
		  AND NOT EXISTS (
				SELECT 1
				FROM topics c
				WHERE c.parent_id = t.id
			)
		ON CONFLICT (user_id, topic_id) DO UPDATE
		SET
			completed = EXCLUDED.completed,
			status = EXCLUDED.status,
			completed_at = CASE
				WHEN EXCLUDED.completed THEN COALESCE(topic_progress.completed_at, now())
				ELSE NULL
			END,
			updated_at = now()
	`, id, owner, completed, status)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return err
}

func (p *Postgres) SetTopicPriority(ctx context.Context, owner, id uuid.UUID, priority string) error {
	tag, err := p.db.Exec(ctx, `
		UPDATE topics t
		SET
			priority = $3,
			updated_at = now()
		FROM roadmaps r
		WHERE t.id = $1
		  AND t.roadmap_id = r.id
		  AND r.owner_id = $2
	`, id, owner, priority)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

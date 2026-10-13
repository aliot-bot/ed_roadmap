package repository

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) ListTopics(ctx context.Context, owner, roadmap uuid.UUID) ([]domain.Topic, error) {
	if _, err := p.GetRoadmap(ctx, owner, roadmap); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `
		SELECT `+topicColumns+`
		FROM topics t
		LEFT JOIN topic_progress p
			ON p.topic_id = t.id
			AND p.user_id = $2
		WHERE t.roadmap_id = $1
		ORDER BY
			t.sort_order,
			t.created_at,
			t.id
	`, roadmap, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Topic{}
	for rows.Next() {
		t, e := scanTopic(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *Postgres) GetTopic(ctx context.Context, owner, id uuid.UUID) (domain.Topic, error) {
	return scanTopic(p.db.QueryRow(ctx, `
		SELECT `+topicColumns+`
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		LEFT JOIN topic_progress p
			ON p.topic_id = t.id
			AND p.user_id = $2
		WHERE t.id = $1
		  AND r.owner_id = $2
	`, id, owner))
}

func (p *Postgres) CreateTopic(ctx context.Context, owner, roadmap uuid.UUID, parent *uuid.UUID, title, desc string, order int) (domain.Topic, error) {
	return p.CreateTopicWithKind(ctx, owner, roadmap, parent, title, desc, order, domain.ElementTopic, domain.TaskNotStarted)
}

func (p *Postgres) CreateTopicWithKind(
	ctx context.Context,
	owner, roadmap uuid.UUID,
	parent *uuid.UUID,
	title, desc string,
	order int,
	kind, taskStatus string,
) (domain.Topic, error) {
	return scanTopic(p.db.QueryRow(ctx, `
		WITH allowed AS (
			SELECT id
			FROM roadmaps
			WHERE id = $1
			  AND owner_id = $2
		), inserted AS (
			INSERT INTO topics (
				roadmap_id,
				parent_id,
				title,
				description,
				sort_order,
				kind,
				task_status
			)
			SELECT
				id,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8
			FROM allowed
			RETURNING
				id,
				roadmap_id,
				parent_id,
				title,
				description,
				sort_order,
				kind,
				task_status,
				created_at,
				updated_at
		)
		SELECT
			i.id,
			i.roadmap_id,
			i.parent_id,
			i.title,
			i.description,
			i.sort_order,
			false,
			'not_started',
			'important',
			i.kind,
			i.task_status,
			i.created_at,
			i.updated_at
		FROM inserted i
	`, roadmap, owner, parent, title, desc, order, kind, taskStatus))
}

func (p *Postgres) CreateTopicWithPriority(
	ctx context.Context,
	owner, roadmap uuid.UUID,
	parent *uuid.UUID,
	title, desc string,
	order int,
	kind, taskStatus, priority string,
) (domain.Topic, error) {
	return scanTopic(p.db.QueryRow(ctx, `
		WITH allowed AS (
			SELECT id
			FROM roadmaps
			WHERE id = $1
			  AND owner_id = $2
		), inserted AS (
			INSERT INTO topics (
				roadmap_id,
				parent_id,
				title,
				description,
				sort_order,
				kind,
				task_status,
				priority
			)
			SELECT
				id,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9
			FROM allowed
			RETURNING
				id,
				roadmap_id,
				parent_id,
				title,
				description,
				sort_order,
				kind,
				task_status,
				priority,
				created_at,
				updated_at
		)
		SELECT
			i.id,
			i.roadmap_id,
			i.parent_id,
			i.title,
			i.description,
			i.sort_order,
			false,
			'not_started',
			i.priority,
			i.kind,
			i.task_status,
			i.created_at,
			i.updated_at
		FROM inserted i
	`, roadmap, owner, parent, title, desc, order, kind, taskStatus, priority))
}

func (p *Postgres) CreateTopicsBatch(ctx context.Context, owner, roadmap uuid.UUID, items []BatchTopic) ([]domain.Topic, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if !ok {
		return nil, domain.ErrNotFound
	}

	out := make([]domain.Topic, 0, len(items))
	for _, item := range items {
		var t domain.Topic
		kind, status, priority := item.Kind, item.TaskStatus, item.Priority

		if kind == "" {
			kind = domain.ElementTopic
		}
		if status == "" {
			status = domain.TaskNotStarted
		}
		if priority == "" {
			priority = domain.PriorityImportant
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO topics (
				id,
				roadmap_id,
				parent_id,
				title,
				description,
				sort_order,
				kind,
				task_status,
				priority
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5,
				$6,
				$7,
				$8,
				$9
			)
			RETURNING
				id,
				roadmap_id,
				parent_id,
				title,
				description,
				sort_order,
				false,
				'not_started',
				priority,
				kind,
				task_status,
				created_at,
				updated_at
		`, item.ID, roadmap, item.ParentID, item.Title, item.Description, item.SortOrder, kind, status, priority).Scan(
			&t.ID,
			&t.RoadmapID,
			&t.ParentID,
			&t.Title,
			&t.Description,
			&t.SortOrder,
			&t.Completed,
			&t.Status,
			&t.Priority,
			&t.Kind,
			&t.TaskStatus,
			&t.CreatedAt,
			&t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Postgres) UpdateTopic(ctx context.Context, owner, id uuid.UUID, parent *uuid.UUID, title, desc string, order int) (domain.Topic, error) {
	return p.UpdateTopicWithKind(ctx, owner, id, parent, title, desc, order, domain.ElementTopic, domain.TaskNotStarted)
}

func (p *Postgres) UpdateTopicWithKind(ctx context.Context, owner, id uuid.UUID, parent *uuid.UUID, title, desc string, order int, kind, taskStatus string) (domain.Topic, error) {
	return scanTopic(p.db.QueryRow(ctx, `
		UPDATE topics t
		SET
			parent_id = $3,
			title = $4,
			description = $5,
			sort_order = $6,
			kind = $7,
			task_status = $8,
			updated_at = now()
		FROM roadmaps r
		WHERE t.id = $1
		  AND t.roadmap_id = r.id
		  AND r.owner_id = $2
		RETURNING
			t.id,
			t.roadmap_id,
			t.parent_id,
			t.title,
			t.description,
			t.sort_order,
			COALESCE(
				(
					SELECT p.completed
					FROM topic_progress p
					WHERE p.topic_id = t.id
					  AND p.user_id = $2
				),
				false
			),
			COALESCE(
				(
					SELECT p.status
					FROM topic_progress p
					WHERE p.topic_id = t.id
					  AND p.user_id = $2
				),
				'not_started'
			),
			t.priority,
			t.kind,
			t.task_status,
			t.created_at,
			t.updated_at
	`, id, owner, parent, title, desc, order, kind, taskStatus))
}

func (p *Postgres) DeleteTopic(ctx context.Context, owner, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `
		DELETE FROM topics t
		USING roadmaps r
		WHERE t.id = $1
		  AND t.roadmap_id = r.id
		  AND r.owner_id = $2
	`, id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

package repository

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"roadmap/backend/internal/domain"
)

type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{db: db}
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func scanRoadmap(row pgx.Row) (r domain.Roadmap, err error) {
	err = row.Scan(
		&r.ID,
		&r.OwnerID,
		&r.Title,
		&r.Description,
		&r.CreatedAt,
		&r.UpdatedAt,
		&r.HasCover,
	)
	return r, notFound(err)
}

func scanTopic(row pgx.Row) (t domain.Topic, err error) {
	err = row.Scan(
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
	if t.Status == "" {
		if t.Completed {
			t.Status = domain.TopicCompleted
		} else {
			t.Status = domain.TopicNotStarted
		}
	}
	if t.Priority == "" {
		t.Priority = domain.PriorityImportant
	}
	return t, notFound(err)
}

const topicColumns = `
	t.id,
	t.roadmap_id,
	t.parent_id,
	t.title,
	t.description,
	t.sort_order,
	COALESCE(p.completed, false),
	COALESCE(
		p.status,
		CASE
			WHEN COALESCE(p.completed, false) THEN 'completed'
			ELSE 'not_started'
		END
	),
	t.priority,
	t.kind,
	t.task_status,
	t.created_at,
	t.updated_at`

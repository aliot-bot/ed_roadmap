package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"roadmap/backend/internal/domain"
)

func scanLearning(row pgx.Row) (s domain.LearningSession, err error) {
	err = row.Scan(
		&s.ID,
		&s.RoadmapID,
		&s.ElementID,
		&s.Kind,
		&s.Status,
		&s.StartedAt,
		&s.UpdatedAt,
		&s.FinishedAt,
		&s.DurationSeconds,
		&s.Note,
	)
	return s, notFound(err)
}

const learningColumns = `
	id,
	roadmap_id,
	element_id,
	kind,
	status,
	started_at,
	updated_at,
	finished_at,
	duration_seconds,
	note`

func (p *Postgres) StartLearningSession(ctx context.Context, owner, roadmap, element uuid.UUID, now time.Time) (domain.LearningSession, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.LearningSession{}, err
	}
	defer tx.Rollback(ctx)

	s, err := scanLearning(tx.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO learning_sessions (
				user_id,
				roadmap_id,
				element_id,
				kind,
				status,
				started_at,
				updated_at
			)
			SELECT
				$1,
				t.roadmap_id,
				t.id,
				t.kind,
				'active',
				$4,
				$4
			FROM topics t
			JOIN roadmaps r
				ON r.id = t.roadmap_id
			WHERE t.id = $3
			  AND t.roadmap_id = $2
			  AND r.owner_id = $1
			RETURNING *
		)
		SELECT `+learningColumns+`
		FROM ins
	`, owner, roadmap, element, now))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.LearningSession{}, domain.ErrConflict
	}

	if err == nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO learning_segments (
				session_id,
				started_at
			)
			VALUES (
				$1,
				$2
			)
		`, s.ID, now)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return s, err
}

func (p *Postgres) GetLearningSession(ctx context.Context, owner, id uuid.UUID) (domain.LearningSession, error) {
	if id == uuid.Nil {
		return scanLearning(p.db.QueryRow(ctx, `
			SELECT `+learningColumns+`
			FROM learning_sessions
			WHERE user_id = $1
			  AND status IN ('active', 'paused')
			ORDER BY updated_at DESC
			LIMIT 1
		`, owner))
	}
	return scanLearning(p.db.QueryRow(ctx, `
		SELECT `+learningColumns+`
		FROM learning_sessions
		WHERE id = $1
		  AND user_id = $2
	`, id, owner))
}

func (p *Postgres) TransitionLearningSession(ctx context.Context, owner, id uuid.UUID, status string, now time.Time, note *string) (domain.LearningSession, error) {
	// The active segment is charged only on pause/finish; paused -> resume charges nothing.
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.LearningSession{}, err
	}
	defer tx.Rollback(ctx)
	s, err := scanLearning(tx.QueryRow(ctx, `
		UPDATE learning_sessions
		SET
			status = $3,
			updated_at = $4,
			finished_at = CASE
				WHEN $3 = 'finished' THEN $4
				ELSE finished_at
			END,
			duration_seconds = duration_seconds + CASE
				WHEN status = 'active' THEN GREATEST(
					0,
					EXTRACT(EPOCH FROM ($4 - updated_at))
				)::bigint
				ELSE 0
			END,
			note = COALESCE($5, note)
		WHERE id = $1
		  AND user_id = $2
		  AND status IN ('active', 'paused')
		RETURNING `+learningColumns, id, owner, status, now, note))
	if err != nil {
		return s, err
	}

	if status == domain.LearningActive {
		_, err = tx.Exec(ctx, `
			INSERT INTO learning_segments (
				session_id,
				started_at
			)
			VALUES (
				$1,
				$2
			)
		`, id, now)
	} else if status == domain.LearningPaused || status == domain.LearningFinished {
		_, err = tx.Exec(ctx, `
			UPDATE learning_segments
			SET ended_at = $2
			WHERE session_id = $1
			  AND ended_at IS NULL
		`, id, now)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return s, err
}

func (p *Postgres) ListLearningSessions(ctx context.Context, owner uuid.UUID, from, to time.Time) ([]domain.LearningSession, error) {
	rows, err := p.db.Query(ctx, `
		SELECT `+learningColumns+`
		FROM learning_sessions
		WHERE user_id = $1
		  AND status = 'finished'
		  AND COALESCE(finished_at, updated_at) >= $2
		  AND started_at < $3
		ORDER BY
			started_at,
			id
	`, owner, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LearningSession{}
	for rows.Next() {
		var s domain.LearningSession
		if err := rows.Scan(
			&s.ID,
			&s.RoadmapID,
			&s.ElementID,
			&s.Kind,
			&s.Status,
			&s.StartedAt,
			&s.UpdatedAt,
			&s.FinishedAt,
			&s.DurationSeconds,
			&s.Note,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}

	return out, rows.Err()
}

func (p *Postgres) ListLearningSegments(ctx context.Context, owner uuid.UUID, from, to time.Time) ([]LearningSegment, error) {
	rows, err := p.db.Query(ctx, `
		SELECT
			s.id,
			s.session_id,
			s.started_at,
			s.ended_at
		FROM learning_segments s
		JOIN learning_sessions l
			ON l.id = s.session_id
		WHERE l.user_id = $1
		  AND l.status = 'finished'
		  AND s.ended_at > $2
		  AND s.started_at < $3
		ORDER BY
			s.started_at,
			s.id
	`, owner, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LearningSegment{}
	for rows.Next() {
		var x LearningSegment
		if err := rows.Scan(&x.ID, &x.SessionID, &x.StartedAt, &x.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

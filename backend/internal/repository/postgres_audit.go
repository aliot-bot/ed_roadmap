package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) Audit(ctx context.Context, user uuid.UUID, actor, action, kind string, id *uuid.UUID, meta map[string]any) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `
		INSERT INTO audit_events (
			user_id,
			actor_type,
			action,
			resource_type,
			resource_id,
			metadata
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5,
			$6
		)
	`, user, actor, action, kind, id, b)
	return err
}

func (p *Postgres) GetIdempotency(ctx context.Context, user uuid.UUID, key string, hash []byte) ([]byte, bool, error) {
	var want, body []byte
	err := p.db.QueryRow(ctx, `
		SELECT
			request_hash,
			response_json
		FROM idempotency_operations
		WHERE user_id = $1
		  AND operation_key = $2
	`, user, key).Scan(&want, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if string(want) != string(hash) {
		return nil, false, domain.ErrConflict
	}
	return body, true, nil
}

func (p *Postgres) PutIdempotency(ctx context.Context, user uuid.UUID, key string, hash, body []byte) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO idempotency_operations (
			user_id,
			operation_key,
			request_hash,
			response_json
		)
		VALUES (
			$1,
			$2,
			$3,
			$4
		)
	`, user, key, hash, body)
	return err
}

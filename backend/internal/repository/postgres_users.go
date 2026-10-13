package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := p.db.QueryRow(ctx, `
		SELECT count(*)
		FROM users
	`).Scan(&n)
	return n, err
}

func (p *Postgres) BootstrapUser(ctx context.Context, login, hash string) (domain.User, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)

	// A transaction-scoped advisory lock closes the check-then-create race even
	// when concurrent bootstrap requests use different logins.
	if _, err = tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(74199271)
	`); err != nil {
		return domain.User{}, err
	}

	var count int
	if err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM users
	`).Scan(&count); err != nil {
		return domain.User{}, err
	}

	if count != 0 {
		return domain.User{}, domain.ErrConflict
	}

	var u domain.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (
			login,
			password_hash
		)
		VALUES (
			$1,
			$2
		)
		RETURNING
			id,
			login,
			display_name,
			created_at
	`, login, hash).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return domain.User{}, fmt.Errorf("bootstrap user: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

func (p *Postgres) CreateUser(ctx context.Context, login, hash string) (domain.User, error) {
	var u domain.User
	err := p.db.QueryRow(ctx, `
		INSERT INTO users (
			login,
			password_hash
		)
		VALUES (
			$1,
			$2
		)
		RETURNING
			id,
			login,
			display_name,
			created_at
	`, login, hash).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (p *Postgres) FindUserByLogin(ctx context.Context, login string) (domain.User, string, error) {
	var u domain.User
	var hash string
	err := p.db.QueryRow(ctx, `
		SELECT
			id,
			login,
			password_hash,
			display_name,
			created_at
		FROM users
		WHERE login = $1
	`, login).Scan(&u.ID, &u.Login, &hash, &u.DisplayName, &u.CreatedAt)
	return u, hash, notFound(err)
}

func (p *Postgres) FindUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	var u domain.User
	err := p.db.QueryRow(ctx, `
		SELECT
			id,
			login,
			display_name,
			created_at
		FROM users
		WHERE id = $1
	`, id).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	return u, notFound(err)
}

func (p *Postgres) CreateSession(ctx context.Context, s domain.Session) error {
	_, err := p.db.Exec(ctx, `
		INSERT INTO sessions (
			id,
			user_id,
			token_hash,
			csrf_hash,
			expires_at
		)
		VALUES (
			$1,
			$2,
			$3,
			$4,
			$5
		)
	`, s.ID, s.UserID, s.TokenHash, s.CSRFHash, s.ExpiresAt)
	return err
}

func (p *Postgres) FindSession(ctx context.Context, hash []byte, now time.Time) (domain.Session, error) {
	var s domain.Session
	err := p.db.QueryRow(ctx, `
		SELECT
			id,
			user_id,
			token_hash,
			csrf_hash,
			expires_at
		FROM sessions
		WHERE token_hash = $1
		  AND expires_at > $2
	`, hash, now).Scan(&s.ID, &s.UserID, &s.TokenHash, &s.CSRFHash, &s.ExpiresAt)
	return s, notFound(err)
}

func (p *Postgres) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := p.db.Exec(ctx, `
		DELETE FROM sessions
		WHERE token_hash = $1
	`, hash)
	return err
}

func (p *Postgres) FindUserByAccessKey(ctx context.Context, hash []byte) (domain.User, error) {
	var u domain.User
	err := p.db.QueryRow(ctx, `
		SELECT
			u.id,
			u.login,
			u.display_name,
			u.created_at
		FROM users u
		JOIN user_access_keys k
			ON k.user_id = u.id
		WHERE k.key_hash = $1
		  AND k.revoked_at IS NULL
	`, hash).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	return u, notFound(err)
}

func (p *Postgres) CreateUserWithAccessKey(ctx context.Context, login, displayName string, _ string, hash []byte) (domain.User, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)

	var u domain.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (
			login,
			display_name,
			password_hash
		)
		VALUES (
			$1,
			$2,
			'!access-key-only!'
		)
		RETURNING
			id,
			login,
			display_name,
			created_at
	`, login, displayName).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return u, err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_access_keys (
			user_id,
			key_hash
		)
		VALUES (
			$1,
			$2
		)
	`, u.ID, hash); err != nil {
		return u, err
	}

	if err = tx.Commit(ctx); err != nil {
		return u, err
	}
	return u, nil
}

func (p *Postgres) RotateAccessKey(ctx context.Context, user uuid.UUID, hash []byte) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		UPDATE user_access_keys
		SET revoked_at = now()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, user); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_access_keys (
			user_id,
			key_hash
		)
		VALUES (
			$1,
			$2
		)
	`, user, hash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RevokeAccessKey(ctx context.Context, user uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `
		UPDATE user_access_keys
		SET revoked_at = now()
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, user)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

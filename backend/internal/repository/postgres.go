package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"roadmap/backend/internal/domain"
)

type Postgres struct{ db *pgxpool.Pool }

func NewPostgres(db *pgxpool.Pool) *Postgres { return &Postgres{db: db} }
func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func scanRoadmap(row pgx.Row) (r domain.Roadmap, err error) {
	err = row.Scan(&r.ID, &r.OwnerID, &r.Title, &r.Description, &r.CreatedAt, &r.UpdatedAt, &r.HasCover)
	return r, notFound(err)
}
func scanTopic(row pgx.Row) (t domain.Topic, err error) {
	err = row.Scan(&t.ID, &t.RoadmapID, &t.ParentID, &t.Title, &t.Description, &t.SortOrder, &t.Completed, &t.Status, &t.Priority, &t.Kind, &t.TaskStatus, &t.CreatedAt, &t.UpdatedAt)
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

const topicColumns = `t.id,t.roadmap_id,t.parent_id,t.title,t.description,t.sort_order,COALESCE(p.completed,false),COALESCE(p.status,CASE WHEN COALESCE(p.completed,false) THEN 'completed' ELSE 'not_started' END),t.priority,t.kind,t.task_status,t.created_at,t.updated_at`

func (p *Postgres) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := p.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
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
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(74199271)`); err != nil {
		return domain.User{}, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return domain.User{}, err
	}
	if count != 0 {
		return domain.User{}, domain.ErrConflict
	}
	var u domain.User
	err = tx.QueryRow(ctx, `INSERT INTO users(login,password_hash) VALUES($1,$2) RETURNING id,login,display_name,created_at`, login, hash).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
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
	err := p.db.QueryRow(ctx, `INSERT INTO users(login,password_hash) VALUES($1,$2) RETURNING id,login,display_name,created_at`, login, hash).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return u, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}
func (p *Postgres) FindUserByLogin(ctx context.Context, login string) (domain.User, string, error) {
	var u domain.User
	var hash string
	err := p.db.QueryRow(ctx, `SELECT id,login,password_hash,display_name,created_at FROM users WHERE login=$1`, login).Scan(&u.ID, &u.Login, &hash, &u.DisplayName, &u.CreatedAt)
	return u, hash, notFound(err)
}
func (p *Postgres) FindUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	var u domain.User
	err := p.db.QueryRow(ctx, `SELECT id,login,display_name,created_at FROM users WHERE id=$1`, id).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	return u, notFound(err)
}
func (p *Postgres) CreateSession(ctx context.Context, s domain.Session) error {
	_, err := p.db.Exec(ctx, `INSERT INTO sessions(id,user_id,token_hash,csrf_hash,expires_at) VALUES($1,$2,$3,$4,$5)`, s.ID, s.UserID, s.TokenHash, s.CSRFHash, s.ExpiresAt)
	return err
}
func (p *Postgres) FindSession(ctx context.Context, hash []byte, now time.Time) (domain.Session, error) {
	var s domain.Session
	err := p.db.QueryRow(ctx, `SELECT id,user_id,token_hash,csrf_hash,expires_at FROM sessions WHERE token_hash=$1 AND expires_at>$2`, hash, now).Scan(&s.ID, &s.UserID, &s.TokenHash, &s.CSRFHash, &s.ExpiresAt)
	return s, notFound(err)
}
func (p *Postgres) DeleteSession(ctx context.Context, hash []byte) error {
	_, err := p.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash)
	return err
}

func (p *Postgres) FindUserByAccessKey(ctx context.Context, hash []byte) (domain.User, error) {
	var u domain.User
	err := p.db.QueryRow(ctx, `SELECT u.id,u.login,u.display_name,u.created_at FROM users u JOIN user_access_keys k ON k.user_id=u.id WHERE k.key_hash=$1 AND k.revoked_at IS NULL`, hash).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	return u, notFound(err)
}
func (p *Postgres) CreateUserWithAccessKey(ctx context.Context, login, displayName string, _ string, hash []byte) (domain.User, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer tx.Rollback(ctx)
	var u domain.User
	err = tx.QueryRow(ctx, `INSERT INTO users(login,display_name,password_hash) VALUES($1,$2,'!access-key-only!') RETURNING id,login,display_name,created_at`, login, displayName).Scan(&u.ID, &u.Login, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_access_keys(user_id,key_hash) VALUES($1,$2)`, u.ID, hash); err != nil {
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
	if _, err = tx.Exec(ctx, `UPDATE user_access_keys SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, user); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO user_access_keys(user_id,key_hash) VALUES($1,$2)`, user, hash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) RevokeAccessKey(ctx context.Context, user uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `UPDATE user_access_keys SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, user)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (p *Postgres) GetLatestLearningReport(ctx context.Context, user uuid.UUID) (domain.LearningReport, error) {
	var r domain.LearningReport
	err := p.db.QueryRow(ctx, `SELECT id,markdown,created_at,status,reason FROM learning_analysis_reports WHERE user_id=$1 ORDER BY created_at DESC LIMIT 1`, user).Scan(&r.ID, &r.Markdown, &r.CreatedAt, &r.Status, &r.Reason)
	return r, notFound(err)
}
func (p *Postgres) SaveLearningReport(ctx context.Context, user uuid.UUID, r domain.LearningReport) (domain.LearningReport, error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	err := p.db.QueryRow(ctx, `INSERT INTO learning_analysis_reports(id,user_id,markdown,status,reason) VALUES($1,$2,$3,$4,$5) RETURNING created_at`, r.ID, user, r.Markdown, r.Status, r.Reason).Scan(&r.CreatedAt)
	return r, err
}
func (p *Postgres) SaveImage(ctx context.Context, user, topic uuid.UUID, mime string, data []byte) (domain.UploadedImage, error) {
	var x domain.UploadedImage
	err := p.db.QueryRow(ctx, `INSERT INTO uploaded_images(user_id,topic_id,mime_type,data) SELECT $1,t.id,$3,$4 FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$2 AND r.owner_id=$1 RETURNING id,topic_id,mime_type,length(data),created_at`, user, topic, mime, data).Scan(&x.ID, &x.TopicID, &x.MIMEType, &x.Size, &x.CreatedAt)
	return x, notFound(err)
}
func (p *Postgres) GetImage(ctx context.Context, user, id uuid.UUID) (domain.UploadedImage, error) {
	var x domain.UploadedImage
	err := p.db.QueryRow(ctx, `SELECT i.id,i.topic_id,i.mime_type,length(i.data),i.created_at,i.data FROM uploaded_images i WHERE i.id=$1 AND i.user_id=$2`, id, user).Scan(&x.ID, &x.TopicID, &x.MIMEType, &x.Size, &x.CreatedAt, &x.Data)
	return x, notFound(err)
}

func (p *Postgres) ListRoadmaps(ctx context.Context, owner uuid.UUID) ([]domain.Roadmap, error) {
	rows, err := p.db.Query(ctx, `SELECT r.id,r.owner_id,r.title,r.description,r.created_at,r.updated_at,EXISTS(SELECT 1 FROM roadmap_covers c WHERE c.roadmap_id=r.id) FROM roadmaps r WHERE r.owner_id=$1 ORDER BY r.updated_at DESC,r.id`, owner)
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
	return scanRoadmap(p.db.QueryRow(ctx, `SELECT r.id,r.owner_id,r.title,r.description,r.created_at,r.updated_at,EXISTS(SELECT 1 FROM roadmap_covers c WHERE c.roadmap_id=r.id) FROM roadmaps r WHERE r.id=$1 AND r.owner_id=$2`, id, owner))
}
func (p *Postgres) CreateRoadmap(ctx context.Context, owner uuid.UUID, title, desc string) (domain.Roadmap, error) {
	return scanRoadmap(p.db.QueryRow(ctx, `INSERT INTO roadmaps(owner_id,title,description) VALUES($1,$2,$3) RETURNING id,owner_id,title,description,created_at,updated_at,false`, owner, title, desc))
}
func (p *Postgres) UpdateRoadmap(ctx context.Context, owner, id uuid.UUID, title, desc *string) (domain.Roadmap, error) {
	return scanRoadmap(p.db.QueryRow(ctx, `UPDATE roadmaps SET title=COALESCE($3,title),description=COALESCE($4,description),updated_at=now() WHERE id=$1 AND owner_id=$2 RETURNING id,owner_id,title,description,created_at,updated_at,EXISTS(SELECT 1 FROM roadmap_covers c WHERE c.roadmap_id=roadmaps.id)`, id, owner, title, desc))
}
func (p *Postgres) DeleteRoadmap(ctx context.Context, owner, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM roadmaps WHERE id=$1 AND owner_id=$2`, id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (p *Postgres) GetRoadmapCover(ctx context.Context, owner, id uuid.UUID) (domain.RoadmapCover, error) {
	var cover domain.RoadmapCover
	err := p.db.QueryRow(ctx, `SELECT c.mime_type,c.data,c.updated_at FROM roadmap_covers c JOIN roadmaps r ON r.id=c.roadmap_id WHERE c.roadmap_id=$1 AND r.owner_id=$2`, id, owner).Scan(&cover.MIMEType, &cover.Data, &cover.UpdatedAt)
	return cover, notFound(err)
}

func (p *Postgres) SetRoadmapCover(ctx context.Context, owner, id uuid.UUID, mime string, data []byte) (domain.RoadmapCover, error) {
	var cover domain.RoadmapCover
	err := p.db.QueryRow(ctx, `INSERT INTO roadmap_covers(roadmap_id,mime_type,data) SELECT r.id,$3,$4 FROM roadmaps r WHERE r.id=$1 AND r.owner_id=$2 ON CONFLICT(roadmap_id) DO UPDATE SET mime_type=EXCLUDED.mime_type,data=EXCLUDED.data,updated_at=now() RETURNING mime_type,data,updated_at`, id, owner, mime, data).Scan(&cover.MIMEType, &cover.Data, &cover.UpdatedAt)
	return cover, notFound(err)
}

func (p *Postgres) DeleteRoadmapCover(ctx context.Context, owner, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM roadmap_covers c USING roadmaps r WHERE c.roadmap_id=$1 AND r.id=c.roadmap_id AND r.owner_id=$2`, id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (p *Postgres) ListTopics(ctx context.Context, owner, roadmap uuid.UUID) ([]domain.Topic, error) {
	if _, err := p.GetRoadmap(ctx, owner, roadmap); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `SELECT `+topicColumns+` FROM topics t LEFT JOIN topic_progress p ON p.topic_id=t.id AND p.user_id=$2 WHERE t.roadmap_id=$1 ORDER BY t.sort_order,t.created_at,t.id`, roadmap, owner)
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
	return scanTopic(p.db.QueryRow(ctx, `SELECT `+topicColumns+` FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id LEFT JOIN topic_progress p ON p.topic_id=t.id AND p.user_id=$2 WHERE t.id=$1 AND r.owner_id=$2`, id, owner))
}
func (p *Postgres) CreateTopic(ctx context.Context, owner, roadmap uuid.UUID, parent *uuid.UUID, title, desc string, order int) (domain.Topic, error) {
	return p.CreateTopicWithKind(ctx, owner, roadmap, parent, title, desc, order, domain.ElementTopic, domain.TaskNotStarted)
}
func (p *Postgres) CreateTopicWithKind(ctx context.Context, owner, roadmap uuid.UUID, parent *uuid.UUID, title, desc string, order int, kind, taskStatus string) (domain.Topic, error) {
	return scanTopic(p.db.QueryRow(ctx, `WITH allowed AS (SELECT id FROM roadmaps WHERE id=$1 AND owner_id=$2), inserted AS (INSERT INTO topics(roadmap_id,parent_id,title,description,sort_order,kind,task_status) SELECT id,$3,$4,$5,$6,$7,$8 FROM allowed RETURNING id,roadmap_id,parent_id,title,description,sort_order,kind,task_status,created_at,updated_at) SELECT i.id,i.roadmap_id,i.parent_id,i.title,i.description,i.sort_order,false,'not_started','important',i.kind,i.task_status,i.created_at,i.updated_at FROM inserted i`, roadmap, owner, parent, title, desc, order, kind, taskStatus))
}
func (p *Postgres) CreateTopicWithPriority(ctx context.Context, owner, roadmap uuid.UUID, parent *uuid.UUID, title, desc string, order int, kind, taskStatus, priority string) (domain.Topic, error) {
	return scanTopic(p.db.QueryRow(ctx, `WITH allowed AS (SELECT id FROM roadmaps WHERE id=$1 AND owner_id=$2), inserted AS (INSERT INTO topics(roadmap_id,parent_id,title,description,sort_order,kind,task_status,priority) SELECT id,$3,$4,$5,$6,$7,$8,$9 FROM allowed RETURNING id,roadmap_id,parent_id,title,description,sort_order,kind,task_status,priority,created_at,updated_at) SELECT i.id,i.roadmap_id,i.parent_id,i.title,i.description,i.sort_order,false,'not_started',i.priority,i.kind,i.task_status,i.created_at,i.updated_at FROM inserted i`, roadmap, owner, parent, title, desc, order, kind, taskStatus, priority))
}
func (p *Postgres) CreateTopicsBatch(ctx context.Context, owner, roadmap uuid.UUID, items []BatchTopic) ([]domain.Topic, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var ok bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roadmaps WHERE id=$1 AND owner_id=$2 FOR UPDATE)`, roadmap, owner).Scan(&ok); err != nil {
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
		err = tx.QueryRow(ctx, `INSERT INTO topics(id,roadmap_id,parent_id,title,description,sort_order,kind,task_status,priority) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id,roadmap_id,parent_id,title,description,sort_order,false,'not_started',priority,kind,task_status,created_at,updated_at`, item.ID, roadmap, item.ParentID, item.Title, item.Description, item.SortOrder, kind, status, priority).Scan(&t.ID, &t.RoadmapID, &t.ParentID, &t.Title, &t.Description, &t.SortOrder, &t.Completed, &t.Status, &t.Priority, &t.Kind, &t.TaskStatus, &t.CreatedAt, &t.UpdatedAt)
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
	return scanTopic(p.db.QueryRow(ctx, `UPDATE topics t SET parent_id=$3,title=$4,description=$5,sort_order=$6,kind=$7,task_status=$8,updated_at=now() FROM roadmaps r WHERE t.id=$1 AND t.roadmap_id=r.id AND r.owner_id=$2 RETURNING t.id,t.roadmap_id,t.parent_id,t.title,t.description,t.sort_order,COALESCE((SELECT p.completed FROM topic_progress p WHERE p.topic_id=t.id AND p.user_id=$2),false),COALESCE((SELECT p.status FROM topic_progress p WHERE p.topic_id=t.id AND p.user_id=$2),'not_started'),t.priority,t.kind,t.task_status,t.created_at,t.updated_at`, id, owner, parent, title, desc, order, kind, taskStatus))
}
func (p *Postgres) DeleteTopic(ctx context.Context, owner, id uuid.UUID) error {
	tag, err := p.db.Exec(ctx, `DELETE FROM topics t USING roadmaps r WHERE t.id=$1 AND t.roadmap_id=r.id AND r.owner_id=$2`, id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}
func (p *Postgres) SetCompletion(ctx context.Context, owner, id uuid.UUID, completed bool) error {
	var exists, saved bool
	err := p.db.QueryRow(ctx, `WITH candidate AS (
  SELECT t.id, NOT EXISTS (SELECT 1 FROM topics child WHERE child.parent_id=t.id) AS is_leaf
  FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id
  WHERE t.id=$1 AND r.owner_id=$2 FOR UPDATE OF t
), saved AS (
	  INSERT INTO topic_progress(user_id,topic_id,completed,status,completed_at,updated_at)
	 SELECT $2,id,$3,CASE WHEN $3 THEN 'completed' ELSE 'not_started' END,CASE WHEN $3 THEN now() ELSE NULL END,now() FROM candidate WHERE is_leaf
	 ON CONFLICT(user_id,topic_id) DO UPDATE SET completed=EXCLUDED.completed,status=EXCLUDED.status,completed_at=EXCLUDED.completed_at,updated_at=now()
  RETURNING topic_id
) SELECT EXISTS(SELECT 1 FROM candidate), EXISTS(SELECT 1 FROM saved)`, id, owner, completed).Scan(&exists, &saved)
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
	tag, err := p.db.Exec(ctx, `INSERT INTO topic_progress(user_id,topic_id,completed,status,completed_at,updated_at)
SELECT $2,t.id,$3,$4,CASE WHEN $3 THEN now() ELSE NULL END,now() FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$1 AND r.owner_id=$2 AND NOT EXISTS(SELECT 1 FROM topics c WHERE c.parent_id=t.id)
ON CONFLICT(user_id,topic_id) DO UPDATE SET completed=EXCLUDED.completed,status=EXCLUDED.status,completed_at=CASE WHEN EXCLUDED.completed THEN COALESCE(topic_progress.completed_at,now()) ELSE NULL END,updated_at=now()`, id, owner, completed, status)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return err
}
func (p *Postgres) SetTopicPriority(ctx context.Context, owner, id uuid.UUID, priority string) error {
	tag, err := p.db.Exec(ctx, `UPDATE topics t SET priority=$3,updated_at=now() FROM roadmaps r WHERE t.id=$1 AND t.roadmap_id=r.id AND r.owner_id=$2`, id, owner, priority)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func scanLearning(row pgx.Row) (s domain.LearningSession, err error) {
	err = row.Scan(&s.ID, &s.RoadmapID, &s.ElementID, &s.Kind, &s.Status, &s.StartedAt, &s.UpdatedAt, &s.FinishedAt, &s.DurationSeconds, &s.Note)
	return s, notFound(err)
}

const learningColumns = `id,roadmap_id,element_id,kind,status,started_at,updated_at,finished_at,duration_seconds,note`

func (p *Postgres) StartLearningSession(ctx context.Context, owner, roadmap, element uuid.UUID, now time.Time) (domain.LearningSession, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.LearningSession{}, err
	}
	defer tx.Rollback(ctx)
	s, err := scanLearning(tx.QueryRow(ctx, `WITH ins AS (INSERT INTO learning_sessions(user_id,roadmap_id,element_id,kind,status,started_at,updated_at) SELECT $1,t.roadmap_id,t.id,t.kind,'active',$4,$4 FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$3 AND t.roadmap_id=$2 AND r.owner_id=$1 RETURNING *) SELECT `+learningColumns+` FROM ins`, owner, roadmap, element, now))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.LearningSession{}, domain.ErrConflict
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO learning_segments(session_id,started_at) VALUES($1,$2)`, s.ID, now)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return s, err
}
func (p *Postgres) GetLearningSession(ctx context.Context, owner, id uuid.UUID) (domain.LearningSession, error) {
	if id == uuid.Nil {
		return scanLearning(p.db.QueryRow(ctx, `SELECT `+learningColumns+` FROM learning_sessions WHERE user_id=$1 AND status IN ('active','paused') ORDER BY updated_at DESC LIMIT 1`, owner))
	}
	return scanLearning(p.db.QueryRow(ctx, `SELECT `+learningColumns+` FROM learning_sessions WHERE id=$1 AND user_id=$2`, id, owner))
}
func (p *Postgres) TransitionLearningSession(ctx context.Context, owner, id uuid.UUID, status string, now time.Time, note *string) (domain.LearningSession, error) {
	// The active segment is charged only on pause/finish; paused -> resume charges nothing.
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return domain.LearningSession{}, err
	}
	defer tx.Rollback(ctx)
	s, err := scanLearning(tx.QueryRow(ctx, `UPDATE learning_sessions SET status=$3,updated_at=$4,finished_at=CASE WHEN $3='finished' THEN $4 ELSE finished_at END,duration_seconds=duration_seconds+CASE WHEN status='active' THEN GREATEST(0,EXTRACT(EPOCH FROM ($4-updated_at)))::bigint ELSE 0 END,note=COALESCE($5,note) WHERE id=$1 AND user_id=$2 AND status IN ('active','paused') RETURNING `+learningColumns, id, owner, status, now, note))
	if err != nil {
		return s, err
	}
	if status == domain.LearningActive {
		_, err = tx.Exec(ctx, `INSERT INTO learning_segments(session_id,started_at) VALUES($1,$2)`, id, now)
	} else if status == domain.LearningPaused || status == domain.LearningFinished {
		_, err = tx.Exec(ctx, `UPDATE learning_segments SET ended_at=$2 WHERE session_id=$1 AND ended_at IS NULL`, id, now)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return s, err
}
func (p *Postgres) ListLearningSessions(ctx context.Context, owner uuid.UUID, from, to time.Time) ([]domain.LearningSession, error) {
	rows, err := p.db.Query(ctx, `SELECT `+learningColumns+` FROM learning_sessions WHERE user_id=$1 AND status='finished' AND COALESCE(finished_at,updated_at)>=$2 AND started_at<$3 ORDER BY started_at,id`, owner, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.LearningSession{}
	for rows.Next() {
		var s domain.LearningSession
		if err := rows.Scan(&s.ID, &s.RoadmapID, &s.ElementID, &s.Kind, &s.Status, &s.StartedAt, &s.UpdatedAt, &s.FinishedAt, &s.DurationSeconds, &s.Note); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (p *Postgres) ListLearningSegments(ctx context.Context, owner uuid.UUID, from, to time.Time) ([]LearningSegment, error) {
	rows, err := p.db.Query(ctx, `SELECT s.id,s.session_id,s.started_at,s.ended_at FROM learning_segments s JOIN learning_sessions l ON l.id=s.session_id WHERE l.user_id=$1 AND l.status='finished' AND s.ended_at>$2 AND s.started_at<$3 ORDER BY s.started_at,s.id`, owner, from, to)
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
func (p *Postgres) GetNote(ctx context.Context, owner, id uuid.UUID) (domain.Note, error) {
	var n domain.Note
	err := p.db.QueryRow(ctx, `SELECT COALESCE(n.content,'') FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id LEFT JOIN topic_notes n ON n.topic_id=t.id AND n.user_id=$2 WHERE t.id=$1 AND r.owner_id=$2`, id, owner).Scan(&n.Content)
	return n, notFound(err)
}
func (p *Postgres) SetNote(ctx context.Context, owner, id uuid.UUID, content string) error {
	tag, err := p.db.Exec(ctx, `INSERT INTO topic_notes(user_id,topic_id,content,updated_at) SELECT $2,t.id,$3,now() FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$1 AND r.owner_id=$2 ON CONFLICT(user_id,topic_id) DO UPDATE SET content=EXCLUDED.content,updated_at=now()`, id, owner, content)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (p *Postgres) ListChecklist(ctx context.Context, owner, id uuid.UUID) ([]domain.ChecklistItem, error) {
	rows, err := p.db.Query(ctx, `SELECT c.id,c.topic_id,c.text,c.completed,c.sort_order FROM topic_checklist_items c JOIN topics t ON t.id=c.topic_id JOIN roadmaps r ON r.id=t.roadmap_id WHERE c.topic_id=$1 AND r.owner_id=$2 ORDER BY c.sort_order,c.id`, id, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ChecklistItem{}
	for rows.Next() {
		var item domain.ChecklistItem
		if err := rows.Scan(&item.ID, &item.TopicID, &item.Text, &item.Completed, &item.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var exists bool
	if err := p.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$1 AND r.owner_id=$2)`, id, owner).Scan(&exists); err != nil || !exists {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	return items, nil
}

func (p *Postgres) SetChecklist(ctx context.Context, owner, id uuid.UUID, inputs []domain.ChecklistItemInput) ([]domain.ChecklistItem, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$1 AND r.owner_id=$2)`, id, owner).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrNotFound
	}
	if _, err = tx.Exec(ctx, `DELETE FROM topic_checklist_items WHERE topic_id=$1`, id); err != nil {
		return nil, err
	}
	items := make([]domain.ChecklistItem, 0, len(inputs))
	for index, input := range inputs {
		itemID := uuid.New()
		if input.ID != nil {
			itemID = *input.ID
		}
		item := domain.ChecklistItem{ID: itemID, TopicID: id, Text: input.Text, Completed: input.Completed, SortOrder: index}
		if _, err = tx.Exec(ctx, `INSERT INTO topic_checklist_items(id,topic_id,text,completed,sort_order) VALUES($1,$2,$3,$4,$5)`, item.ID, item.TopicID, item.Text, item.Completed, item.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

// AppendChecklist adds unchecked items without replacing the existing checklist.
func (p *Postgres) AppendChecklist(ctx context.Context, owner, id uuid.UUID, texts []string) ([]domain.ChecklistItem, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$1 AND r.owner_id=$2)`, id, owner).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrNotFound
	}
	var nextOrder int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(sort_order) + 1, 0) FROM topic_checklist_items WHERE topic_id=$1`, id).Scan(&nextOrder); err != nil {
		return nil, err
	}
	items := make([]domain.ChecklistItem, 0, len(texts))
	for index, text := range texts {
		item := domain.ChecklistItem{ID: uuid.New(), TopicID: id, Text: text, Completed: false, SortOrder: nextOrder + index}
		if _, err = tx.Exec(ctx, `INSERT INTO topic_checklist_items(id,topic_id,text,completed,sort_order) VALUES($1,$2,$3,$4,$5)`, item.ID, item.TopicID, item.Text, item.Completed, item.SortOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (p *Postgres) GetTopicDocument(ctx context.Context, owner, id uuid.UUID) (domain.TopicDocument, error) {
	var d domain.TopicDocument
	err := p.db.QueryRow(ctx, `SELECT COALESCE(d.content,''),d.created_at,d.updated_at FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id LEFT JOIN topic_documents d ON d.topic_id=t.id WHERE t.id=$1 AND r.owner_id=$2`, id, owner).Scan(&d.Content, &d.CreatedAt, &d.UpdatedAt)
	return d, notFound(err)
}

func (p *Postgres) SetTopicDocument(ctx context.Context, owner, id uuid.UUID, content string) (domain.TopicDocument, error) {
	var d domain.TopicDocument
	err := p.db.QueryRow(ctx, `INSERT INTO topic_documents(topic_id,content) SELECT t.id,$3 FROM topics t JOIN roadmaps r ON r.id=t.roadmap_id WHERE t.id=$1 AND r.owner_id=$2 ON CONFLICT(topic_id) DO UPDATE SET content=EXCLUDED.content,updated_at=now() RETURNING content,created_at,updated_at`, id, owner, content).Scan(&d.Content, &d.CreatedAt, &d.UpdatedAt)
	return d, notFound(err)
}

func (p *Postgres) ListDependencies(ctx context.Context, owner, roadmap uuid.UUID) ([]domain.Dependency, error) {
	if _, err := p.GetRoadmap(ctx, owner, roadmap); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `SELECT predecessor_topic_id,successor_topic_id FROM topic_dependencies WHERE roadmap_id=$1 ORDER BY predecessor_topic_id,successor_topic_id`, roadmap)
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
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roadmaps WHERE id=$1 AND owner_id=$2 FOR UPDATE)`, roadmap, owner).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	if _, err = tx.Exec(ctx, `DELETE FROM topic_dependencies WHERE roadmap_id=$1`, roadmap); err != nil {
		return err
	}
	for _, d := range deps {
		if _, err = tx.Exec(ctx, `INSERT INTO topic_dependencies(roadmap_id,predecessor_topic_id,successor_topic_id) VALUES($1,$2,$3)`, roadmap, d.PredecessorTopicID, d.SuccessorTopicID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) ListLayouts(ctx context.Context, owner, roadmap uuid.UUID) ([]domain.Layout, error) {
	if _, err := p.GetRoadmap(ctx, owner, roadmap); err != nil {
		return nil, err
	}
	rows, err := p.db.Query(ctx, `SELECT topic_id,x,y,collapsed FROM topic_layouts WHERE roadmap_id=$1 ORDER BY topic_id`, roadmap)
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
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roadmaps WHERE id=$1 AND owner_id=$2 FOR UPDATE)`, roadmap, owner).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	for _, l := range items {
		tag, e := tx.Exec(ctx, `INSERT INTO topic_layouts(roadmap_id,topic_id,x,y,collapsed,updated_at) SELECT $1,t.id,$3,$4,$5,now() FROM topics t WHERE t.id=$2 AND t.roadmap_id=$1 ON CONFLICT(roadmap_id,topic_id) DO UPDATE SET x=EXCLUDED.x,y=EXCLUDED.y,collapsed=EXCLUDED.collapsed,updated_at=now()`, roadmap, l.TopicID, l.X, l.Y, l.Collapsed)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrValidation
		}
	}
	return tx.Commit(ctx)
}
func (p *Postgres) Audit(ctx context.Context, user uuid.UUID, actor, action, kind string, id *uuid.UUID, meta map[string]any) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = p.db.Exec(ctx, `INSERT INTO audit_events(user_id,actor_type,action,resource_type,resource_id,metadata) VALUES($1,$2,$3,$4,$5,$6)`, user, actor, action, kind, id, b)
	return err
}
func (p *Postgres) GetIdempotency(ctx context.Context, user uuid.UUID, key string, hash []byte) ([]byte, bool, error) {
	var want, body []byte
	err := p.db.QueryRow(ctx, `SELECT request_hash,response_json FROM idempotency_operations WHERE user_id=$1 AND operation_key=$2`, user, key).Scan(&want, &body)
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
	_, err := p.db.Exec(ctx, `INSERT INTO idempotency_operations(user_id,operation_key,request_hash,response_json) VALUES($1,$2,$3,$4)`, user, key, hash, body)
	return err
}

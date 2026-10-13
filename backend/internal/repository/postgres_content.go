package repository

import (
	"context"

	"github.com/google/uuid"
	"roadmap/backend/internal/domain"
)

func (p *Postgres) GetNote(ctx context.Context, owner, id uuid.UUID) (domain.Note, error) {
	var n domain.Note
	err := p.db.QueryRow(ctx, `
		SELECT COALESCE(n.content, '')
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		LEFT JOIN topic_notes n
			ON n.topic_id = t.id
			AND n.user_id = $2
		WHERE t.id = $1
		  AND r.owner_id = $2
	`, id, owner).Scan(&n.Content)
	return n, notFound(err)
}

func (p *Postgres) SetNote(ctx context.Context, owner, id uuid.UUID, content string) error {
	tag, err := p.db.Exec(ctx, `
		INSERT INTO topic_notes (
			user_id,
			topic_id,
			content,
			updated_at
		)
		SELECT
			$2,
			t.id,
			$3,
			now()
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		WHERE t.id = $1
		  AND r.owner_id = $2
		ON CONFLICT (user_id, topic_id) DO UPDATE
		SET
			content = EXCLUDED.content,
			updated_at = now()
	`, id, owner, content)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

func (p *Postgres) ListChecklist(ctx context.Context, owner, id uuid.UUID) ([]domain.ChecklistItem, error) {
	rows, err := p.db.Query(ctx, `
		SELECT
			c.id,
			c.topic_id,
			c.text,
			c.completed,
			c.sort_order
		FROM topic_checklist_items c
		JOIN topics t
			ON t.id = c.topic_id
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		WHERE c.topic_id = $1
		  AND r.owner_id = $2
		ORDER BY
			c.sort_order,
			c.id
	`, id, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.ChecklistItem{}
	for rows.Next() {
		var item domain.ChecklistItem
		if err := rows.Scan(
			&item.ID,
			&item.TopicID,
			&item.Text,
			&item.Completed,
			&item.SortOrder,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	var exists bool
	if err := p.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM topics t
			JOIN roadmaps r
				ON r.id = t.roadmap_id
			WHERE t.id = $1
			  AND r.owner_id = $2
		)
	`, id, owner).Scan(&exists); err != nil || !exists {
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
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM topics t
			JOIN roadmaps r
				ON r.id = t.roadmap_id
			WHERE t.id = $1
			  AND r.owner_id = $2
		)
	`, id, owner).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrNotFound
	}

	if _, err = tx.Exec(ctx, `
		DELETE FROM topic_checklist_items
		WHERE topic_id = $1
	`, id); err != nil {
		return nil, err
	}
	items := make([]domain.ChecklistItem, 0, len(inputs))
	for index, input := range inputs {
		itemID := uuid.New()
		if input.ID != nil {
			itemID = *input.ID
		}
		item := domain.ChecklistItem{
			ID:        itemID,
			TopicID:   id,
			Text:      input.Text,
			Completed: input.Completed,
			SortOrder: index,
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO topic_checklist_items (
				id,
				topic_id,
				text,
				completed,
				sort_order
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5
			)
		`, item.ID, item.TopicID, item.Text, item.Completed, item.SortOrder); err != nil {
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
	if err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM topics t
			JOIN roadmaps r
				ON r.id = t.roadmap_id
			WHERE t.id = $1
			  AND r.owner_id = $2
		)
	`, id, owner).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, domain.ErrNotFound
	}

	var nextOrder int
	if err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order) + 1, 0)
		FROM topic_checklist_items
		WHERE topic_id = $1
	`, id).Scan(&nextOrder); err != nil {
		return nil, err
	}

	items := make([]domain.ChecklistItem, 0, len(texts))
	for index, text := range texts {
		item := domain.ChecklistItem{
			ID:        uuid.New(),
			TopicID:   id,
			Text:      text,
			Completed: false,
			SortOrder: nextOrder + index,
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO topic_checklist_items (
				id,
				topic_id,
				text,
				completed,
				sort_order
			)
			VALUES (
				$1,
				$2,
				$3,
				$4,
				$5
			)
		`, item.ID, item.TopicID, item.Text, item.Completed, item.SortOrder); err != nil {
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
	err := p.db.QueryRow(ctx, `
		SELECT
			COALESCE(d.content, ''),
			d.created_at,
			d.updated_at
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		LEFT JOIN topic_documents d
			ON d.topic_id = t.id
		WHERE t.id = $1
		  AND r.owner_id = $2
	`, id, owner).Scan(&d.Content, &d.CreatedAt, &d.UpdatedAt)
	return d, notFound(err)
}

func (p *Postgres) SetTopicDocument(ctx context.Context, owner, id uuid.UUID, content string) (domain.TopicDocument, error) {
	var d domain.TopicDocument
	err := p.db.QueryRow(ctx, `
		INSERT INTO topic_documents (
			topic_id,
			content
		)
		SELECT
			t.id,
			$3
		FROM topics t
		JOIN roadmaps r
			ON r.id = t.roadmap_id
		WHERE t.id = $1
		  AND r.owner_id = $2
		ON CONFLICT (topic_id) DO UPDATE
		SET
			content = EXCLUDED.content,
			updated_at = now()
		RETURNING
			content,
			created_at,
			updated_at
	`, id, owner, content).Scan(&d.Content, &d.CreatedAt, &d.UpdatedAt)
	return d, notFound(err)
}

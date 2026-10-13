-- +goose Up
-- Long-form Markdown documents are separate from the short per-topic notes.
CREATE TABLE IF NOT EXISTS topic_documents (
    topic_id uuid PRIMARY KEY REFERENCES topics(id) ON DELETE CASCADE,
    content text NOT NULL DEFAULT '' CHECK (length(content) <= 200000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS topic_documents;

-- Small owner-scoped cover images make roadmap cards visually distinct without
-- inflating every roadmap/list response with binary data.
CREATE TABLE roadmap_covers (
    roadmap_id uuid PRIMARY KEY REFERENCES roadmaps(id) ON DELETE CASCADE,
    mime_type text NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png', 'image/gif', 'image/webp')),
    data bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 5242880),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX roadmap_covers_updated_idx ON roadmap_covers(updated_at);

-- +goose Down
DROP TABLE IF EXISTS roadmap_covers;

CREATE TABLE IF NOT EXISTS topic_checklist_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  topic_id uuid NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  text text NOT NULL CHECK (length(btrim(text)) BETWEEN 1 AND 500),
  completed boolean NOT NULL DEFAULT false,
  sort_order integer NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS topic_checklist_items_topic_idx ON topic_checklist_items(topic_id,sort_order,id);

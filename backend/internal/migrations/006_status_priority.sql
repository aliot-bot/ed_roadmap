-- Topic status is user-controlled; completed is retained for compatibility.
ALTER TABLE topics ADD COLUMN IF NOT EXISTS priority text NOT NULL DEFAULT 'important';
ALTER TABLE topics DROP CONSTRAINT IF EXISTS topics_priority_check;
ALTER TABLE topics ADD CONSTRAINT topics_priority_check CHECK (priority IN ('critical','important','optional','skippable'));
ALTER TABLE topic_progress ADD COLUMN IF NOT EXISTS status text;
UPDATE topic_progress SET status = CASE WHEN completed THEN 'completed' ELSE 'not_started' END WHERE status IS NULL;
ALTER TABLE topic_progress ALTER COLUMN status SET DEFAULT 'not_started';
ALTER TABLE topic_progress ALTER COLUMN status SET NOT NULL;
ALTER TABLE topic_progress DROP CONSTRAINT IF EXISTS topic_progress_status_check;
ALTER TABLE topic_progress ADD CONSTRAINT topic_progress_status_check CHECK (status IN ('not_started','in_progress','needs_review','completed'));
CREATE INDEX IF NOT EXISTS topics_priority_idx ON topics(priority);
CREATE INDEX IF NOT EXISTS topic_progress_status_idx ON topic_progress(user_id,status);
CREATE TABLE IF NOT EXISTS learning_segments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), session_id uuid NOT NULL REFERENCES learning_sessions(id) ON DELETE CASCADE,
  started_at timestamptz NOT NULL, ended_at timestamptz,
  CHECK (ended_at IS NULL OR ended_at >= started_at)
);
CREATE UNIQUE INDEX IF NOT EXISTS learning_segments_open_idx ON learning_segments(session_id) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS learning_segments_time_idx ON learning_segments(started_at,ended_at);
-- Preserve historical confirmed time. Pause boundaries were not stored by the
-- legacy schema, so this conservative backfill places each session's confirmed
-- duration immediately after its start (never inventing extra minutes).
INSERT INTO learning_segments(session_id,started_at,ended_at)
SELECT l.id,l.started_at,l.started_at + (l.duration_seconds * interval '1 second')
FROM learning_sessions l
WHERE l.status='finished' AND l.duration_seconds > 0
  AND NOT EXISTS (SELECT 1 FROM learning_segments s WHERE s.session_id=l.id);

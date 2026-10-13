ALTER TABLE topics ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'topic';
ALTER TABLE topics ADD COLUMN IF NOT EXISTS task_status text NOT NULL DEFAULT 'not_started';
ALTER TABLE topics DROP CONSTRAINT IF EXISTS topics_kind_check;
ALTER TABLE topics ADD CONSTRAINT topics_kind_check CHECK (kind IN ('topic','task'));
ALTER TABLE topics DROP CONSTRAINT IF EXISTS topics_task_status_check;
ALTER TABLE topics ADD CONSTRAINT topics_task_status_check CHECK (task_status IN ('not_started','in_progress','completed'));
CREATE TABLE learning_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  roadmap_id uuid NOT NULL REFERENCES roadmaps(id) ON DELETE CASCADE,
  element_id uuid NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  kind text NOT NULL CHECK (kind IN ('topic','task')),
  status text NOT NULL CHECK (status IN ('active','paused','finished')),
  started_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  duration_seconds bigint NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
  note text NOT NULL DEFAULT '' CHECK (length(note) <= 2000)
);
CREATE UNIQUE INDEX learning_sessions_one_open_idx ON learning_sessions(user_id) WHERE status IN ('active','paused');
CREATE INDEX learning_sessions_activity_idx ON learning_sessions(user_id, started_at, finished_at);

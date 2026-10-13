ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS user_access_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key_hash bytea NOT NULL UNIQUE, created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz
);
CREATE INDEX IF NOT EXISTS user_access_keys_active_idx ON user_access_keys(key_hash) WHERE revoked_at IS NULL;
CREATE TABLE IF NOT EXISTS learning_analysis_reports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  markdown text NOT NULL DEFAULT '', status text NOT NULL CHECK (status IN ('ready','unavailable','failed')),
  reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS learning_analysis_reports_latest_idx ON learning_analysis_reports(user_id, created_at DESC);
CREATE TABLE IF NOT EXISTS uploaded_images (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  topic_id uuid NOT NULL REFERENCES topics(id) ON DELETE CASCADE, mime_type text NOT NULL,
  data bytea NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS uploaded_images_owner_idx ON uploaded_images(user_id, created_at DESC);

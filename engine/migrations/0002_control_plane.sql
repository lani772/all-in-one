-- HMAC-authenticated Go control-plane API and direct-to-object-store source uploads.
-- Apply after 0000_projects.sql and 0001_deployments.sql.

ALTER TABLE projects ADD COLUMN IF NOT EXISTS slug TEXT;
UPDATE projects
SET slug = 'project-' || substr(md5(id), 1, 10)
WHERE slug IS NULL OR btrim(slug) = '';
ALTER TABLE projects ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS projects_slug_unique_idx ON projects(slug);

CREATE TABLE IF NOT EXISTS source_uploads (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  owner_id TEXT NOT NULL,
  object_key TEXT NOT NULL UNIQUE,
  original_name TEXT NOT NULL,
  size_bytes BIGINT NOT NULL CHECK (size_bytes BETWEEN 1 AND 104857600),
  status TEXT NOT NULL CHECK (status IN ('pending', 'uploaded', 'failed', 'expired')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  uploaded_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS source_uploads_project_created_idx ON source_uploads(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS source_uploads_expiry_idx ON source_uploads(expires_at) WHERE status = 'pending';

-- HMAC and timestamp validation happen before a nonce is inserted.
CREATE TABLE IF NOT EXISTS auth_nonces (
  nonce TEXT PRIMARY KEY CHECK (nonce ~ '^[a-f0-9]{48}$'),
  owner_id TEXT NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS auth_nonces_expiry_idx ON auth_nonces(expires_at);

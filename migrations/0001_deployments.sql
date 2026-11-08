-- Apply after the core projects table exists. A deployment becomes current
-- only when the repository transaction commits all rows and updates the pointer.
ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS current_deployment_id TEXT;

CREATE TABLE IF NOT EXISTS deployments (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  owner_id TEXT NOT NULL,
  build_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('ready', 'superseded', 'deleted')),
  manifest_key TEXT NOT NULL UNIQUE,
  total_bytes BIGINT NOT NULL CHECK (total_bytes >= 0),
  file_count INTEGER NOT NULL CHECK (file_count > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (project_id, id)
);

CREATE INDEX IF NOT EXISTS deployments_owner_created_idx
  ON deployments(owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS deployments_project_created_idx
  ON deployments(project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS deployment_files (
  deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
  path TEXT NOT NULL,
  object_key TEXT NOT NULL UNIQUE,
  size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
  sha256 TEXT NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
  content_type TEXT NOT NULL,
  PRIMARY KEY (deployment_id, path)
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'projects'::regclass
      AND conname = 'projects_current_deployment_fk'
  ) THEN
    ALTER TABLE projects
      ADD CONSTRAINT projects_current_deployment_fk
      FOREIGN KEY (id, current_deployment_id)
      REFERENCES deployments(project_id, id)
      DEFERRABLE INITIALLY DEFERRED;
  END IF;
END $$;

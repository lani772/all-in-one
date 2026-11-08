-- The Go engine uses a dedicated database. Keep this schema separate from
-- dashboard-managed metadata until the production integration is explicitly wired.
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS projects_owner_created_idx
  ON projects(owner_id, created_at DESC);

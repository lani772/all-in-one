-- PostgreSQL-backed build job state and transactional outbox for Redis Streams.
-- Apply after migrations 0000-0002.

CREATE TABLE IF NOT EXISTS build_jobs (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  source_upload_id TEXT NOT NULL REFERENCES source_uploads(id),
  owner_id TEXT NOT NULL,
  source_object_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','inspecting','inspected','failed','cancelled')),
  inspection_result JSONB,
  error_message TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS build_jobs_owner_created_idx ON build_jobs(owner_id,created_at DESC);
CREATE INDEX IF NOT EXISTS build_jobs_project_created_idx ON build_jobs(project_id,created_at DESC);
CREATE INDEX IF NOT EXISTS build_jobs_queue_idx ON build_jobs(status,created_at) WHERE status IN ('queued','inspecting');

CREATE TABLE IF NOT EXISTS queue_outbox (
  id BIGSERIAL PRIMARY KEY,
  job_id TEXT NOT NULL UNIQUE REFERENCES build_jobs(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  dispatched_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS queue_outbox_pending_idx ON queue_outbox(id) WHERE dispatched_at IS NULL;

CREATE TABLE IF NOT EXISTS engine_workers (
  worker_id TEXT PRIMARY KEY,
  mode TEXT NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

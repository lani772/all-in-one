-- Sealed source uploads and transactional inspection outbox.
ALTER TABLE source_uploads ADD COLUMN IF NOT EXISTS sealed_object_key TEXT, ADD COLUMN IF NOT EXISTS sha256 TEXT;
ALTER TABLE source_uploads DROP CONSTRAINT IF EXISTS source_uploads_status_check;
ALTER TABLE source_uploads ADD CONSTRAINT source_uploads_status_check CHECK (status IN ('pending','uploaded','sealed','inspecting','inspected','failed','expired'));
CREATE UNIQUE INDEX IF NOT EXISTS source_uploads_sealed_key_unique_idx ON source_uploads(sealed_object_key) WHERE sealed_object_key IS NOT NULL;
CREATE TABLE IF NOT EXISTS inspection_jobs (
 id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
 upload_id TEXT NOT NULL UNIQUE REFERENCES source_uploads(id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL, source_object_key TEXT NOT NULL,
 source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[a-f0-9]{64}$'),
 source_size_bytes BIGINT NOT NULL CHECK (source_size_bytes BETWEEN 1 AND 104857600),
 status TEXT NOT NULL CHECK (status IN ('queued','inspecting','succeeded','failed')),
 attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0), log_text TEXT NOT NULL DEFAULT '',
 result JSONB, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS inspection_jobs_owner_created_idx ON inspection_jobs(owner_id,created_at DESC);
CREATE INDEX IF NOT EXISTS inspection_jobs_recovery_idx ON inspection_jobs(status,updated_at);
CREATE TABLE IF NOT EXISTS job_outbox (
 id BIGSERIAL PRIMARY KEY, job_id TEXT NOT NULL REFERENCES inspection_jobs(id) ON DELETE CASCADE,
 event_type TEXT NOT NULL, payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 published_at TIMESTAMPTZ, attempts INTEGER NOT NULL DEFAULT 0, UNIQUE(job_id,event_type)
);
CREATE INDEX IF NOT EXISTS job_outbox_pending_idx ON job_outbox(id) WHERE published_at IS NULL;

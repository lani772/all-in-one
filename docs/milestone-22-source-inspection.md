# Milestone 22 — immutable source sealing and asynchronous inspection

## Scope

This milestone connects source finalization to a real, non-executing inspection pipeline. It does not install dependencies or build customer projects.

## Lifecycle

1. Dashboard uploads a ZIP using a short-lived S3 POST policy for `sources/{project}/{upload}/source.zip`.
2. The authenticated API locks the pending upload row and verifies the object byte count.
3. The API copies the source to `sealed-sources/{project}/{upload}/source.zip`, a namespace never exposed to browser write policies.
4. The API reads the sealed copy, calculates SHA-256, and in one PostgreSQL transaction marks the upload sealed, creates an inspection job, and inserts an outbox event.
5. The worker publishes outbox events to Redis Streams. PostgreSQL remains authoritative if Redis publishing or a worker process crashes.
6. A consumer-group worker downloads the sealed object, checks the stored size and SHA-256, safely extracts it, and inspects package metadata.
7. Job status, attempts, result JSON and logs are persisted in PostgreSQL. Owners read them through the authenticated API.

The outbox is at-least-once: a crash after Redis XADD but before the outbox row is marked published can create duplicate messages. Worker claiming and job-state checks make duplicate delivery harmless. Pending stream messages are reclaimed after five minutes.

## ZIP security

- Reject absolute paths, drive-letter paths, backslashes, NUL bytes, dot segments, duplicate/conflicting paths, symlinks and special files.
- Enforce archive entry count, per-file expanded size, total expanded size and compression-ratio ceilings.
- Create files with exclusive creation and restrictive permissions in a fresh temporary directory.
- Read each entry to EOF so ZIP CRC errors surface before inspection.
- The worker verifies the immutable source hash before extraction.

## API contract

- `POST /api/v1/projects/{id}/source-finalize` returns HTTP 202 with `inspectionJobId`, sealed object key, SHA-256 and `status: queued`.
- Repeated finalization returns the existing inspection job for already sealed/inspecting/inspected uploads.
- `GET /api/v1/inspection-jobs/{id}` is owner-scoped and returns status, attempts, logs, timestamps and optional result.
- Build endpoints continue to return HTTP 503; no build job is fabricated.

## Configuration

Required worker variables: `DATABASE_URL`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `REDIS_ADDR`. Optional controls: `MAX_SOURCE_BYTES`, `MAX_ARCHIVE_ENTRIES`, `MAX_EXPANDED_BYTES`, `MAX_COMPRESSION_RATIO`, `INSPECTION_TIMEOUT_SECONDS`.

Apply `engine/migrations/0003_source_inspection.sql` after migrations 0000–0002. Redis 7 is recommended because the worker uses consumer groups and XAUTOCLAIM.

## Not yet delivered

- Actual package installation and isolated build execution.
- Dependency cache or audited egress proxy.
- Build cancellation/retries and streaming logs.
- Artifact publishing from worker outputs.
- Static hosting gateway and project route isolation.
- Next.js static-export build and independently managed SSR runtime.
- Live multi-service integration testing and production deployment.

# Milestone 22 — Redis job queue and secure inspection worker

## Implemented

1. The API creates a `build_jobs` row and `queue_outbox` row in one PostgreSQL transaction after verifying an owner-scoped finalized source upload.
2. A separate worker dispatches pending outbox rows to a Redis Stream consumer group.
3. Redis delivery is at-least-once. The worker conditionally claims jobs in PostgreSQL, making duplicate stream entries harmless.
4. The worker downloads the private source ZIP, safely extracts it, and inspects `package.json`, framework dependencies, lockfiles, scripts and entrypoints.
5. Job state and inspection results are persisted in PostgreSQL. The dashboard can poll status and cancel queued jobs.
6. Worker heartbeats cause `/readyz` to report `buildExecution: inspection-only` while a worker is active.

## API changes

| Method | Route | Behavior |
|---|---|---|
| POST | `/api/v1/projects/{id}/builds` | Creates a DB job + outbox event and returns 202 |
| GET | `/api/v1/builds/{id}` | Returns owner-scoped status and inspection result |
| POST | `/api/v1/builds/{id}/cancel` | Cancels queued jobs; returns 409 once inspection has started |

A job reaches `inspected`, not `succeeded`. This is not a live deployment.

## ZIP and resource limits

Defaults:
- compressed source ZIP: 20 MiB
- entries: 10,000
- expanded file: 100 MiB
- total expanded content: 500 MiB
- compression ratio: 100:1
- extraction root and files use restrictive permissions
- absolute paths, traversal, Windows drive/alternate-stream paths, backslashes, NULs, symlinks and special files are rejected
- duplicate entries and overwrites are rejected
- no project code, package lifecycle hook or build command is executed

The temporary directory is removed after each job. The worker does not mount a Docker socket and does not receive build secrets.

## Reliability

- PostgreSQL is the source of truth; Redis is delivery transport.
- The transactional outbox prevents committed jobs from being silently lost when Redis is unavailable.
- If XADD succeeds but the outbox transaction fails, a duplicate stream event may be produced; the database claim is idempotent.
- Pending Redis messages are reclaimed after a four-minute idle window. Processing is limited to three minutes.
- Inspection failures are persisted before the stream event is acknowledged.
- A crashed worker can leave a job in `inspecting`; stale-job reclamation permits retry after the lease window.

## Run

From `engine/`:

```bash
go mod tidy
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0003_build_queue.sql
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/api ./cmd/worker
go run ./cmd/api
# In another process:
go run ./cmd/worker
```

Redis 6.2+ is required for `XAUTOCLAIM`. Configure `REDIS_ADDR` and optional `REDIS_PASSWORD`.

## Not implemented

- Dependency installation and actual project builds.
- Hardened disposable Docker build hosts and controlled package-registry egress/cache.
- Cancellation after inspection starts.
- Build log streaming and retention.
- Artifact publisher integration.
- Public static gateway, domain routing and rollback.
- Next.js SSR runtime.
- Live PostgreSQL/Redis/MinIO integration tests and production deployment.

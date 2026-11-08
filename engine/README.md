# All-in-One Hosting — Go control API and artifact storage

The Go engine contains the Milestone 19 immutable artifact publisher, Milestone 20 PostgreSQL/S3 adapters, and the Milestone 21 authenticated project/source control API.

## Milestone 21 — control API

- `cmd/api` starts the Go HTTP API with bounded timeouts and graceful shutdown.
- `internal/auth` verifies the dashboard's HMAC request headers, enforces a five-minute timestamp window, and consumes one-time 192-bit nonces in PostgreSQL.
- `internal/controlapi` creates projects and unique slugs, issues short-lived size-bounded source ZIP POST policies, verifies finalized object sizes, and lists owner-scoped deployments.
- `internal/objectstore/source.go` adds the private source-upload namespace and MinIO presigned PUT/stat operations.
- `migrations/0002_control_plane.sql` adds project slugs, source upload metadata and replay-protection nonces.

The API deliberately returns HTTP 503 for build creation/status/cancellation until a real Redis consumer and isolated Docker build worker are connected. It does not create a fake queued job.

## Configure and run

Requirements: Go 1.23+, PostgreSQL, and a private S3-compatible bucket (MinIO works for local development).

```bash
cp .env.example .env
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0000_projects.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_deployments.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0002_control_plane.sql
go mod tidy
go run ./cmd/api
```

The API listens on `:8080` by default. `/healthz` is liveness; `/readyz` checks PostgreSQL and the configured S3 bucket. Readiness reports `buildExecution: not-connected` until the build worker exists.

Build the container with:

```bash
docker build -f Dockerfile.api -t allinone-hosting-api:local .
```

## Dashboard connection

Set encrypted dashboard secrets:
- `ENGINE_API_URL` — HTTPS origin of the Go API (no path/query/fragment).
- `AUTH_HMAC_SECRET` — same random secret as the Go API environment, at least 32 bytes.

The dashboard backend signs requests; the browser never receives this secret. Keep the Go API behind HTTPS and a network policy that only permits the trusted dashboard backend. Do not trust `X-Authenticated-User` from public clients.

Configure S3/MinIO CORS to allow `POST` from the dashboard origin with the `Content-Type` request header. The POST policy enforces the exact declared object size and `application/zip` type. The bucket must remain private.

## API

- `GET /healthz`
- `GET /readyz`
- `POST /api/v1/projects`
- `POST /api/v1/projects/{id}/source-upload`
- `POST /api/v1/projects/{id}/source-finalize`
- `GET /api/v1/projects/{id}/deployments`
- Build endpoints return explicit 503 until build execution is connected.

All `/api/v1/*` routes require the HMAC headers created by the dashboard backend.

## Tests and verification

```bash
gofmt -w cmd internal
go test ./...
go test -race ./...
go vet ./...
```

A full build requires the Go module dependencies to be downloadable. The local auth/control API helper tests use standard-library packages; object storage and PostgreSQL integration require dependencies and live services for end-to-end verification.

## Milestone 22 — Redis job queue and inspection worker

- `POST /api/v1/projects/{id}/builds` writes a build job and PostgreSQL outbox row in one transaction, then returns HTTP 202.
- The separate worker dispatches outbox rows to a Redis Stream consumer group. Duplicate stream entries are safe because jobs are conditionally claimed in PostgreSQL.
- The worker downloads the private ZIP, applies extraction limits, rejects traversal and symlinks, and inspects package metadata without executing tenant code.
- `GET /api/v1/builds/{id}` returns owner-scoped status. Queued jobs can be cancelled; active inspection cannot yet be cancelled.
- Worker heartbeats report `buildExecution: inspection-only` in readiness.

Apply the new migration after 0000-0002:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0003_build_queue.sql
```

Run the API and worker separately:

```bash
go run ./cmd/api
go run ./cmd/worker
```

The worker deliberately does not run dependency installation, lifecycle scripts, package build commands or uploaded executable code. `inspected` is not a successful deployment. Actual builds remain disabled until the hardened Docker executor and controlled dependency cache are integrated. Redis 6.2+ is required for pending-message reclamation.

# All-in-One Hosting — Go control API and inspection worker

The engine contains the immutable static artifact publisher, PostgreSQL/S3 adapters, authenticated control API, and Redis Streams inspection worker.

## Milestone 22 — sealed uploads and asynchronous source inspection

- Finalized uploads are copied from the presigned `sources/` namespace into the service-owned `sealed-sources/` namespace. The browser never receives a write policy for sealed objects.
- The API hashes the sealed copy, then atomically records its immutable key, source digest, inspection job and transactional outbox event in PostgreSQL.
- A separate worker dispatches outbox events to Redis Streams and consumes them through a consumer group. Duplicate messages are safe; abandoned pending messages are reclaimed after five minutes.
- The worker downloads the sealed ZIP, verifies SHA-256 and byte count, extracts with traversal/type/size/compression-ratio checks, and inspects framework/package metadata.
- Inspection status, result and logs are stored in PostgreSQL and are visible only to the authenticated owner.
- No customer JavaScript, package lifecycle scripts, build commands or network requests are executed.

## Apply migrations

Apply these in order to the dedicated engine database:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0000_projects.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_deployments.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0002_control_plane.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0003_source_inspection.sql
```

## Configure and run

Requirements: Go 1.23+, PostgreSQL, Redis 7 (Streams and XAUTOCLAIM), and a private S3-compatible bucket.

Set variables in `.env` based on `.env.example`. Run API and worker separately:

```bash
go mod tidy
go run ./cmd/api
go run ./cmd/worker
```

Build images:

```bash
docker build -f Dockerfile.api -t allinone-hosting-api:local .
docker build -f Dockerfile.worker -t allinone-hosting-worker:local .
```

The API remains behind HTTPS and a private network policy. Configure S3/MinIO CORS to allow POST from the dashboard origin with Content-Type. Keep the bucket private. Grant the browser access only through the short-lived, size-bounded POST policy; never expose S3 credentials.

## API additions

- `POST /api/v1/projects/{id}/source-finalize` copies the upload to the sealed namespace, computes SHA-256, creates an inspection job and returns HTTP 202 with `inspectionJobId`.
- `GET /api/v1/inspection-jobs/{id}` returns owner-scoped inspection status, attempts, logs and framework metadata.
- Build creation/status/cancellation still return HTTP 503. Inspection is not a deployment build.

## Worker limits

Defaults: 20 MiB compressed source, 10,000 ZIP entries, 100 MiB per expanded file, 512 MiB total expanded bytes, compression ratio 200:1 and five-minute inspection timeout. Tune conservatively for the worker host.

The worker does not execute source code. It should run with a read-only root filesystem, restricted temporary storage, no Docker socket and no build credentials. The S3 identity used by the worker should be read-only for sealed sources.

## Tests

```bash
gofmt -w cmd internal
go mod tidy
go test ./...
go test -race ./...
go vet ./...
```

Live PostgreSQL, Redis and MinIO integration still requires a provisioned environment. The CI workflow runs Go tests and builds the worker container but does not claim a production deployment.

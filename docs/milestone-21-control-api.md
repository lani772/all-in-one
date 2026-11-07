# Milestone 21 — authenticated control API and source upload lifecycle

## Implemented

- Go `net/http` API entrypoint with bounded request timeouts and graceful shutdown.
- HMAC authentication matching the current dashboard canonical string: `timestamp\nnonce\nuserId\nMETHOD\npath`.
- Five-minute clock-skew validation, one-time 192-bit nonces, constant-time signature comparison, and PostgreSQL replay protection.
- Project creation with normalized, globally unique URL slugs.
- Short-lived private S3/MinIO presigned PUT URLs for ZIP source archives.
- Source finalization that verifies the stored object exists and has the exact declared byte size.
- Owner-scoped deployment listing.
- Readiness separates PostgreSQL and object storage from build execution.

## API contract

All `/api/v1/*` routes are signed by the dashboard backend. `X-Authenticated-User` is trusted only after HMAC verification. Keep the engine behind HTTPS and a private network policy.

| Method | Route | Result |
|---|---|---|
| GET | `/healthz` | Liveness |
| GET | `/readyz` | PostgreSQL/S3 readiness; build execution is `not-connected` |
| POST | `/api/v1/projects` | Creates project and returns `projectId`, `slug` |
| POST | `/api/v1/projects/{id}/source-upload` | Returns a 15-minute presigned ZIP PUT URL |
| POST | `/api/v1/projects/{id}/source-finalize` | Verifies object existence and exact byte count |
| GET | `/api/v1/projects/{id}/deployments` | Lists the owner's deployment metadata |
| POST | `/api/v1/projects/{id}/builds` | Explicit 503 until Redis consumer/build worker is connected |
| GET | `/api/v1/builds/{id}` | Explicit 503 until build worker is connected |
| POST | `/api/v1/builds/{id}/cancel` | Explicit 503 until build worker is connected |

The current dashboard signs method, path, user, timestamp and nonce. HTTPS protects the request body in transit. A future protocol version should include the body SHA-256 in the HMAC canonical string before exposing the engine across an untrusted network.

## Apply

From `engine/`:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0000_projects.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_deployments.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0002_control_plane.sql
```

Set `DATABASE_URL`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION`, `S3_USE_TLS`, and a random `AUTH_HMAC_SECRET` of at least 32 bytes. The HMAC secret must match the encrypted dashboard secret. `LISTEN_ADDR` defaults to `:8080`; `MAX_SOURCE_BYTES` defaults to 20 MiB.

Configure the private S3/MinIO bucket CORS policy to allow `PUT` from the dashboard origin with the `Content-Type` header. Keep the bucket private; presigned URLs grant access to one object and expire after 15 minutes.

## Deliberate limitations

- Source upload accepts ZIP archives only. The dashboard packages selected individual files into a ZIP before upload.
- Finalization verifies object existence and size, not ZIP contents. Secure archive extraction and inspection must happen in a separate worker.
- No Redis queue consumer, dependency installer, Docker build worker, artifact publisher integration, or public hosting gateway is wired by this API yet.
- Build endpoints return 503 and do not create fake queued jobs.
- No Next.js SSR runtime is included.
- Apply migrations before starting the API; the API does not mutate the schema automatically.

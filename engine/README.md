# Go hosting engine source

This directory contains the repository-native static artifact pipeline.

## Milestone 19 — artifact publication core

`internal/artifact` creates deterministic manifests for completed static build outputs, rejects symlinks and non-regular files, blocks common secret files, enforces file-count and byte limits, hashes files with SHA-256, and uploads under immutable deployment-specific object keys.

## Milestone 20 — production storage and database adapters

- `internal/objectstore` implements the private S3-compatible object store using MinIO's Go SDK. It validates deployment-scoped object keys, requires an already provisioned bucket, and never changes bucket ACL or policy.
- `internal/deployment` implements the PostgreSQL repository using pgx through `database/sql`. It locks the project row, verifies ownership, inserts the deployment and file rows, supersedes the prior ready deployment, and switches the current deployment pointer in one serializable transaction.
- `migrations/0000_projects.sql` creates the Go engine's dedicated projects table. Apply it before `0001_deployments.sql`.

### Configure

Copy `.env.example` to `.env` and supply production credentials through a secret manager. Do not commit `.env`, credentials, or tenant build secrets.

Provision the bucket as private before starting the service. For a local MinIO instance, for example:

```bash
mc mb --ignore-existing local/hosting-artifacts
mc anonymous set none local/hosting-artifacts
```

Run migrations in order against the dedicated engine database:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0000_projects.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_deployments.sql
```

Then verify the module:

```bash
go mod tidy
gofmt -w internal
go test ./...
go test -race ./...
go vet ./...
```

### Integration boundary

The adapters are not yet wired into a public HTTP route or build worker. The caller must derive `ownerID` from verified authentication, never from a client-supplied header. Publish only a completed, immutable output directory after the build process has stopped. Do not expose the S3 bucket publicly. The hosting gateway must resolve a project slug to its current deployment and serve only manifest-listed keys. Next.js SSR requires a separate runtime service and is not handled by this static artifact store.

The database transaction serializes publishes per project through `SELECT ... FOR UPDATE`. If COMMIT returns a network error, the publisher must retain uploaded objects and let reconciliation inspect database references before cleanup; deleting them immediately could break a successfully committed deployment.

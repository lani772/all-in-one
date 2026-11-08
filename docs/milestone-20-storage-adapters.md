# Milestone 20 — production storage and database adapters

## Delivered

1. A MinIO/S3 adapter for immutable deployment object keys.
2. Object-key validation that rejects absolute paths, traversal, backslashes, and keys outside the expected project/deployment namespace.
3. A PostgreSQL adapter using a serializable transaction and a project-row lock.
4. Owner verification before any deployment metadata is written.
5. Atomic insertion of deployment/file records, superseding the old ready deployment, and switching the active deployment pointer.
6. A dedicated core projects migration so migration order is explicit.
7. Unit tests for key validation and manifest metadata invariants.

## Commands

From `engine/`:

```bash
go mod tidy
gofmt -w internal
go test ./...
go test -race ./...
go vet ./...
```

Apply migrations in order to a dedicated engine database:

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0000_projects.sql
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f migrations/0001_deployments.sql
```

## Security and operational assumptions

- S3 bucket is provisioned ahead of time and private. The adapter does not set public ACLs or bucket policies.
- Credentials are supplied through environment injection or a secret manager; they are never passed to tenant build containers.
- `ownerID` is produced by a trusted authentication layer.
- Artifact directories are immutable after the build worker exits.
- A COMMIT error can have an ambiguous outcome. Retain object data until a DB-aware orphan reconciler confirms it is unreferenced.
- This adapter is not exposed as an HTTP endpoint yet and does not itself run builds.

# Go hosting engine source

This directory currently contains the first repository-native Go module: the immutable static-artifact publishing package. The complete API, queue, worker, S3 adapter and gateway source tree referenced by earlier milestone packages is **not yet present in this repository**; do not treat this package alone as a running deployment engine.

## Milestone 19 — static artifact publication core

`internal/artifact` builds a deterministic manifest for a completed static build output, rejects symlinks and non-regular files, blocks common secret files, enforces file-count/per-file/total-size limits, hashes every file with SHA-256, and uploads files under immutable deployment-scoped object keys. It then uploads a JSON manifest and calls a repository interface whose contract requires an atomic metadata transaction and active-deployment pointer switch. If an upload fails, already written object keys are deleted best-effort. If the database commit reports an error, immutable objects are retained because a network failure can make commit outcome ambiguous; an orphan sweeper must query database references before deleting anything.

The object-store adapter must target a **private bucket** and consume exactly the declared bytes. The repository adapter must verify that the authenticated owner owns the project and atomically persist deployment rows, file rows and the current deployment pointer. Those production adapters and the worker integration are still pending.

### Commands

```bash
gofmt -w internal/artifact/*.go
go test ./...
go test -race ./...
go vet ./...
```

Apply `migrations/0001_deployments.sql` only after the engine's core `projects` table has been created. Review the schema and back up the database before applying it.

### Security boundary

Only publish a completed, immutable build output directory from a trusted worker after the build process has stopped. Do not point this publisher at user source directories. Never make the object bucket public. The hosting gateway should resolve a slug to the current deployment ID, then serve only manifest-listed object keys; it must not accept arbitrary object keys from a request path. Use a separate runtime service for Next.js SSR—this static publisher is not an SSR deployment path.

# All-in-One Hosting

Self-hostable frontend hosting platform with a React + TypeScript dashboard and a separate Go control plane/build engine.

- `dashboard/` — AppDeploy React dashboard source and authenticated backend-for-frontend integration.
- `engine/` — repository-native Go modules and migrations. The static artifact publication core is implemented; the complete API, worker, S3 adapter and gateway source tree still needs to be imported and integrated.
- `docs/` — production acceptance requirements and milestone implementation notes.

Live dashboard: https://all-in-one-hosting-u3d98s.v2.appdeploy.ai/

The dashboard backend signs Go API requests with HMAC-SHA256 using encrypted AppDeploy secrets. The browser never receives the HMAC key. Individual source files are packaged into ZIP in the browser and uploaded to private object storage using a presigned PUT URL.

The Go service must be deployed separately and configured before end-to-end builds work. Sandbox execution remains disabled by default. Generate a real go.sum for the full engine when its dependencies are imported, run CI and complete production acceptance before exposing the platform to customers.

## Current repository milestone

Milestone 19 adds `engine/internal/artifact`: deterministic static artifact manifests, SHA-256 verification, secret-file and symlink rejection, resource limits, immutable object keys, and a publisher interface with rollback/reconciliation behavior. See `docs/milestone-19-artifact-pipeline.md`. This package is not yet connected to the dashboard, build worker, production object store, PostgreSQL repository, or public gateway.

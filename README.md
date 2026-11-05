# All-in-One Hosting

Self-hostable frontend hosting platform with a React + TypeScript dashboard and a separate Go control plane/build engine.

## Components

- `dashboard/` — source snapshot of the AppDeploy React dashboard and authenticated backend-for-frontend integration.
- `engine/` — Go API, build worker, edge synchronizer, Redis Streams queue, PostgreSQL metadata, private S3-compatible object storage, static gateway and Caddy integration. Import the Go source from the supplied Milestone 18 source package before building; see `engine/README.md`.
- `docs/` — production acceptance requirements.

## Dashboard

Live dashboard: https://all-in-one-hosting-u3d98s.v2.appdeploy.ai/

The dashboard backend signs Go API requests with HMAC-SHA256 using encrypted AppDeploy secrets. The browser never receives the HMAC key. Individual source files are packaged into ZIP in the browser and uploaded to private object storage using a presigned PUT URL.

## Required dashboard secrets

`ENGINE_API_URL`, `AUTH_HMAC_SECRET`, and optionally `PUBLIC_HOSTING_BASE_URL`. Enter values only through AppDeploy's secure secret-entry workflow; never commit them.

## Important release status

The dashboard is deployed, but the Go engine must be deployed separately and configured before end-to-end builds work. Sandbox execution remains disabled by default. Generate a real `go.sum`, run CI and complete the production acceptance checklist before exposing the platform to customers.

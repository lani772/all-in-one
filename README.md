# All-in-One Hosting

Self-hostable frontend hosting platform with a React + TypeScript dashboard and a separate Go control plane/build engine.

- `dashboard/` — AppDeploy React dashboard source and authenticated backend-for-frontend integration.
- `engine/` — Go API, build worker, edge synchronizer, Redis Streams, PostgreSQL, private S3-compatible storage, static gateway and Caddy integration.
- `docs/` — production acceptance requirements.

Live dashboard: https://all-in-one-hosting-u3d98s.v2.appdeploy.ai/

The dashboard backend signs Go API requests with HMAC-SHA256 using encrypted AppDeploy secrets. The browser never receives the HMAC key. Individual source files are packaged into ZIP in the browser and uploaded to private object storage using a presigned PUT URL.

The Go service must be deployed separately and configured before end-to-end builds work. Sandbox execution remains disabled by default. Generate a real `go.sum`, run CI and complete production acceptance before exposing the platform to customers.

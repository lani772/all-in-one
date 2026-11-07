# All-in-One Hosting

Self-hostable frontend hosting platform with a React + TypeScript dashboard and a separate Go control plane/build engine.

- `dashboard/` — AppDeploy React dashboard source and authenticated backend-for-frontend integration.
- `engine/` — repository-native Go artifact pipeline, private MinIO/S3 adapter, PostgreSQL deployment repository and ordered migrations.
- `docs/` — milestone implementation notes and production acceptance requirements.

Live dashboard: https://all-in-one-hosting-u3d98s.v2.appdeploy.ai/

The dashboard backend signs Go API requests with HMAC-SHA256 using encrypted AppDeploy secrets. The browser never receives the HMAC key. Individual source files are packaged into ZIP in the browser and uploaded to private object storage using a presigned PUT URL.

## Current repository milestone

Milestone 20 adds concrete MinIO/S3 and PostgreSQL adapters for the Milestone 19 immutable static-artifact publisher. The database adapter verifies project ownership and atomically writes deployment metadata while switching the active deployment pointer. A GitHub Actions workflow checks Go formatting, tests, race behavior and vetting.

**This repository still does not contain the complete Go API, Redis queue consumer, isolated build worker or public hosting gateway.** The adapters are not yet connected to a build job or HTTP route. Sandbox execution remains disabled by default. No live build or public deployment is claimed. Next.js SSR requires a separate runtime service.

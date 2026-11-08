# All-in-One Hosting

Self-hostable frontend hosting platform with a React + TypeScript + Vite dashboard and a separate Go control plane.

- `dashboard/` — React + TypeScript + Vite dashboard and authenticated backend-for-frontend integration.
- `engine/` — Go API, PostgreSQL metadata, Redis Streams inspection worker, private MinIO/S3 source/artifact storage, and immutable static artifact publisher.
- `docs/` — milestone implementation notes and production acceptance requirements.

Live dashboard: https://all-in-one-hosting-u3d98s.v2.appdeploy.ai/

## Current repository milestone

Milestone 22 adds a sealed-source lifecycle and asynchronous inspection pipeline. Finalized uploads are copied to a server-owned object key and hashed. A PostgreSQL transactional outbox feeds Redis Streams; a separate worker safely extracts ZIPs, detects framework/package metadata, and stores owner-scoped status and logs.

**Inspection is not a deployment build.** No customer code or package scripts are executed. Build creation remains disabled until the dedicated isolated Docker builder, dependency cache/egress policy, build logs, artifact publication and public gateway are completed and verified. Next.js SSR remains a separate runtime service.

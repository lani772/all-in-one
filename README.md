# All-in-One Hosting

Self-hostable frontend hosting platform with a React + TypeScript dashboard and a separate Go control plane.

- `dashboard/` — React + TypeScript + Vite dashboard and authenticated backend-for-frontend integration.
- `engine/` — Go control API, HMAC authentication, PostgreSQL metadata, private MinIO/S3 source/artifact storage adapters, and immutable static artifact publication core.
- `docs/` — milestone implementation notes and production acceptance requirements.

Live dashboard: https://all-in-one-hosting-u3d98s.v2.appdeploy.ai/

## Current repository milestone

Milestone 21 adds the Go control API used by the dashboard's existing signed-engine routes. It creates projects and slugs, issues short-lived private ZIP upload URLs, verifies finalized source objects by exact byte count, and lists owner-scoped deployment metadata. HMAC timestamp and one-time nonce checks protect control-plane routes.

**Build execution remains disabled.** The API returns explicit HTTP 503 responses for build requests until a real Redis consumer and isolated Docker worker are connected. Source finalization is not archive inspection, and no public hosting deployment is claimed. Next.js SSR remains a separate future runtime.

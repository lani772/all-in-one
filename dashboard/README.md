# Dashboard source

This directory contains the latest AppDeploy dashboard source snapshot. It uses `@appdeploy/client` and `@appdeploy/sdk`, which are injected by AppDeploy and are not a standalone npm dependency.

Required encrypted app secrets:
- `ENGINE_API_URL`: HTTPS origin of the Go API, no path/query.
- `AUTH_HMAC_SECRET`: same 32+ character secret configured on the Go API.
- `PUBLIC_HOSTING_BASE_URL`: optional public gateway origin for generated project links.

The canonical HMAC payload is `timestamp\\nnonce\\nuserId\\nMETHOD\\n/api/v1/path`. Headers: `X-Authenticated-User`, `X-Auth-Timestamp`, `X-Auth-Nonce`, `X-Auth-Signature`.

The browser sends only ZIP bytes to the short-lived presigned object-storage URL. Configure storage CORS to allow `PUT` from the dashboard origin with `Content-Type`; do not use wildcard origins in production.

Individual files are packaged as a store-only ZIP; existing ZIP archives pass through unchanged. Jobs are shown as inspection-only when `ENABLE_SANDBOX_BUILDS=false`; no public site is claimed until a real build succeeds.

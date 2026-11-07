# Milestone 19 — immutable static artifact pipeline

## Scope

This milestone introduced a repository-native Go package for publishing completed static build outputs as immutable deployment versions. Milestone 20 now supplies the MinIO/S3 and PostgreSQL adapters.

## Publication sequence

1. A trusted worker completes a build and stops all child processes.
2. The worker gives the publisher an output directory that is no longer writable by the build process.
3. The publisher walks the output, rejects symlinks and special files, blocks likely secret files, and enforces file-count and byte limits.
4. Each regular file is SHA-256 hashed and assigned a deterministic MIME type and immutable object key under `sites/{project}/deployments/{deployment}/files/`.
5. Files and `manifest.json` are uploaded to a pre-provisioned private S3-compatible bucket.
6. The PostgreSQL adapter verifies project ownership and atomically inserts deployment/file metadata and updates the project's current deployment pointer.
7. An upload error triggers best-effort deletion of objects already written. A database commit error retains objects because commit outcome can be ambiguous after a network failure; an orphan sweeper must query database references before deleting anything.

## Important invariants

- Deployment IDs and project IDs are opaque, validated identifiers—not slugs or user paths.
- Manifest paths are normalized relative paths and are never used as arbitrary bucket keys by the gateway.
- Every file's size and SHA-256 are verified again while uploading to detect output changes after manifest creation.
- The gateway must serve only paths present in the active deployment manifest and must set `Content-Type`, `Content-Length`, `ETag`/digest and safe cache headers from trusted metadata.
- `index.html` should be served for the root. SPA fallback behavior must be an explicit project setting, not a blanket fallback that masks missing assets.
- Old deployment objects remain immutable for rollback until retention cleanup explicitly removes them.
- Next.js SSR is not handled here; it needs a separate runtime deployment type and isolation policy.

## Remaining integration work

- Connect the publisher to the trusted build worker after a sandboxed build succeeds.
- Add a database-aware orphan sweeper and deployment retention jobs.
- Implement the manifest-aware hosting gateway and prevent path traversal, open redirects and cross-project object access.
- Add rollback API that changes the current deployment pointer transactionally.
- Import/reconcile the complete API and worker source tree before production deployment.

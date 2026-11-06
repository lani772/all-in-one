# Milestone 19 — immutable static artifact pipeline

## Scope

This milestone introduces a repository-native Go package for publishing completed static build outputs as immutable deployment versions. It does not claim to run builds or serve sites.

## Publication sequence

1. A trusted worker completes a build and stops all child processes.
2. The worker gives the publisher an output directory that is no longer writable by the build process.
3. The publisher walks the output, rejects symlinks and special files, blocks likely secret files, and enforces file-count and byte limits.
4. Each regular file is SHA-256 hashed and assigned a deterministic MIME type and immutable object key under `sites/{project}/{deployment}/files/`.
5. Files and `manifest.json` are uploaded to a private object bucket.
6. A repository adapter atomically inserts the deployment and file metadata and updates the project's current deployment pointer.
7. An upload error triggers best-effort deletion of objects already written. A database commit error retains objects because commit outcome can be ambiguous after a network failure; an orphan sweeper must query database references before deleting unreferenced objects. A periodic sweeper is also required for process crashes.

## Important invariants

- Deployment IDs and project IDs are opaque, validated identifiers—not slugs or user paths.
- Manifest paths are normalized relative paths and are never used as arbitrary bucket keys by the gateway.
- Every file's size and SHA-256 are verified again while uploading to detect output changes after manifest creation.
- The gateway must serve only paths present in the active deployment manifest and must set `Content-Type`, `Content-Length`, `ETag`/digest and safe cache headers from trusted metadata.
- `index.html` should be served for the root. SPA fallback behavior must be an explicit project setting, not a blanket fallback that masks missing assets.
- Old deployment objects remain immutable for rollback until retention cleanup explicitly removes them.
- Next.js SSR is not handled here; it needs a separate runtime deployment type and isolation policy.

## Remaining integration work

- Implement the S3-compatible `ObjectStore` adapter with multipart upload and private ACL/bucket policy.
- Implement the PostgreSQL `DeploymentRepository` transaction and owner check.
- Call the publisher only after a sandboxed worker succeeds.
- Add orphan cleanup and deployment retention jobs.
- Implement a gateway that resolves current deployment metadata and prevents path traversal, open redirects and cross-project object access.
- Add rollback API that changes the current deployment pointer transactionally.
- Import the complete API/worker source package and reconcile it with this module before production deployment.

# Production acceptance checklist

## Current repository status — Milestone 19

The repository now contains a tested, standard-library-only Go static artifact publication core under `engine/internal/artifact`. Its object-store and database interfaces still require production adapters, and it is not yet called by a build worker. The full Go API/worker/edge-sync source package is still absent from `engine/`; the `engine/README.md` records this explicitly. Do not enable public deployments based on the artifact package alone.

- [ ] Import and reconcile the complete Go engine API/worker/edge-sync source tree into `engine/`.
- [ ] Implement the private S3-compatible `ObjectStore` adapter and the transactional PostgreSQL `DeploymentRepository` for `engine/internal/artifact`.
- [ ] Connect artifact publication to a completed, isolated build worker; ensure the output directory is immutable before publishing.
- [ ] Implement an orphan-object sweeper that checks database references before deleting objects left by process crashes or ambiguous commits.
- [ ] Implement a manifest-aware static gateway and transactional rollback API.

## Existing production acceptance requirements

- [ ] Generate and commit a real `go.sum` using `cd engine && go mod tidy` on a network-enabled machine after all engine dependencies are present.
- [ ] Run `gofmt`, `go vet ./...`, `go test -race ./...` and GitHub Actions.
- [ ] Build API, worker and edge-sync images; pin deployed images by immutable digest.
- [ ] Provision PostgreSQL, Redis, MinIO/S3 and backups with private network access and non-default credentials.
- [ ] Set the same strong `AUTH_HMAC_SECRET` in the Go API and dashboard's encrypted AppDeploy secret store.
- [ ] Verify request signing, replay rejection, rate limiting and owner isolation.
- [ ] Configure restrictive object-storage CORS for the dashboard origin and verify presigned PUT + finalize.
- [ ] Validate ZIP traversal, symlink, expansion-ratio and archive-size rejection.
- [ ] Provision a dedicated remote Docker host with mutual TLS; never mount the host Docker socket into tenant containers or the public API.
- [ ] Keep sandbox builds disabled until the remote host and pinned offline builder image pass isolation tests.
- [ ] Verify private Caddy admin access, DNS ownership verification and ACME issue/renewal on a test domain.
- [ ] Exercise deploy, failed build, cancellation, rollback, artifact cleanup and backup restore.
- [ ] Confirm `/readyz`, `/metrics`, alerts, log retention and incident runbooks.
- [ ] Run a controlled upload-to-live-site acceptance test before production traffic.

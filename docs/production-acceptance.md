# Production acceptance checklist

- [ ] Import the complete Go engine source tree into `engine/` from the Milestone 18 source package.
- [ ] Generate and commit a real `go.sum` using `cd engine && go mod tidy` on a network-enabled machine.
- [ ] Run `gofmt`, `go vet ./...`, `go test -race ./...` and GitHub Actions.
- [ ] Build API, worker and edge-sync images; pin deployed images by immutable digest.
- [ ] Provision PostgreSQL, Redis, MinIO/S3 and backups with private network access and non-default credentials.
- [ ] Set the same strong `AUTH_HMAC_SECRET` in the Go API and the dashboard's encrypted AppDeploy secret store.
- [ ] Verify request signing, replay rejection, rate limiting and owner isolation.
- [ ] Configure restrictive object-storage CORS for the dashboard origin and verify presigned PUT + finalize.
- [ ] Validate ZIP traversal, symlink, expansion-ratio and archive-size rejection.
- [ ] Provision a dedicated remote Docker host with mutual TLS; never mount the host Docker socket into tenant containers or the public API.
- [ ] Keep sandbox builds disabled until the remote host and pinned offline builder image pass isolation tests.
- [ ] Verify Caddy admin access is private, DNS ownership verification works, and ACME certificates issue and renew on a test domain.
- [ ] Exercise deploy, failed build, cancellation, rollback, superseded deployment cleanup and backup restore.
- [ ] Confirm `/readyz`, `/metrics`, alerts, log retention and incident runbooks.
- [ ] Run a controlled upload-to-live-site acceptance test before production traffic.

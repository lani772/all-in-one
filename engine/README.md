# Go hosting engine

The full Go source tree is maintained in the Milestone 18 source package delivered with this release. Extract its `engine/` directory here before building. The source package includes API, worker, edge-sync, queue, archive validation, artifact publishing, gateway, TLS configuration, PostgreSQL schema, Dockerfiles, Compose and CI workflows.

Module path: `github.com/lani772/all-in-one/engine`.

After importing the source tree:

```bash
cp .env.example .env
# Set strong credentials and a matching AUTH_HMAC_SECRET.
docker compose up -d --build
```

`ENABLE_SANDBOX_BUILDS=false` is the safe default. Do not enable it until a dedicated remote Docker daemon with mutual TLS and a pinned offline builder image has passed isolation tests. See `../docs/production-acceptance.md`.

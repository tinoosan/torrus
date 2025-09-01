# OpenAPI

The OpenAPI definition lives at [`index.yaml`](../index.yaml).

## Updating
- Edit `index.yaml` to reflect API changes.
- Keep examples in sync with handlers and data models.
- Future tooling will generate client SDKs from this file.

## Conventions
- Read‑only fields such as `name` and `files` are marked `readOnly`.
- The spec is strict JSON: unknown fields are rejected.
- Versioned under `/v1`; unversioned paths are limited to `/healthz` and
  `/readyz` for infrastructure probes. The `/metrics` endpoint is not
  included in the OpenAPI specification.

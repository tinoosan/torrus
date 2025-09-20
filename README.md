# Torrus API

Torrus is a lightweight download orchestration service. It abstracts downloaders like aria2 behind a simple REST API so apps can request torrents, magnet links, or direct downloads without talking to the downloader directly.

## Status
🚧 **Under active development** — APIs, data models, and behavior are subject to change.

## Features
- Create and list downloads
- Update desired state (Active / Resume / Paused / Cancelled)
- Retrieve download details (idempotent create via fingerprint)
- Pluggable downloader (aria2 adapter, noop for dev)
- Auth via bearer token; structured logs; Prometheus metrics; health/readiness endpoints

## Use Cases

### Media server automation
Use Torrus as a backend for Plex, Emby, or Jellyfin to automatically fetch torrents, magnet links, or direct downloads for your library.

### General download orchestration
Abstract aria2 or other downloaders behind a REST API so dashboards, UIs, or scripts can control downloads without talking to the downloader directly.

### Microservice integration
Run Torrus in Docker, Kubernetes, or other containerized environments where companion services request downloads via API instead of managing aria2 themselves.

### Future Extensions
- CLI interface for scripting.
- Event-driven workflows (e.g., triggering jobs after a download completes).
- Multi-user or multi-tenant support.

## Versioning Policy
All API endpoints are explicitly versioned starting with **v1**.
Future breaking changes will be introduced under a new version (e.g., `/v2/...`).
Unversioned endpoints are disabled by default to encourage consistent usage.

- Current version: **v1**
- Unversioned routes: return `404` (or may optionally redirect to `/v1/...`)
- Futre health check endpoints (e.g., `/healthz`) will remain unversioned

## Quickstart

Run locally with Go:
```
go run ./cmd
```

Run with Docker (debug image):
```
docker compose up --build
```
This starts Torrus and aria2. Call `POST /v1/downloads` to create a download.

## API Overview

Torrus exposes a JSON-over-HTTP interface:

- Authentication – All endpoints except `/healthz`, `/readyz`, `/metrics` require `Authorization: Bearer <token>`.
- Content type – `Content-Type: application/json`; unknown fields and >1 MiB bodies are rejected.
- Logging – Structured logs with method, path, status, duration, bytes; `X-Request-ID` supported.
- Metrics – Prometheus at `/metrics`; health at `/healthz`; readiness at `/readyz`.

### Correlation IDs
All HTTP requests support an optional `X-Request-ID` header for log correlation. If you provide one, the same value appears in server logs as `request_id` and is echoed back in the response header. If you omit it, the server generates a UUID and returns it.

Example:

```
curl -H 'X-Request-ID: my-debug-id' http://localhost:9090/v1/downloads -i
```

Use this value to trace activity across handlers, services, repos, and any downloader work started by the request.

### Deletion Semantics & Safety
When deleting a download with `deleteFiles=true`, Torrus applies strict safeguards:

- Base directory is never deleted: only paths strictly under `targetPath/` are eligible. If `targetPath` is empty, only absolute, cleaned paths are considered.
- Path safety: all candidate paths are normalized and must remain within `targetPath/`.
- Sidecar ownership rules: control files like `.aria2` and `.torrent` are removed only when ownership is proven:
  - Exact-name match: files named exactly `base/<dl.Name>.aria2` and, for torrents, `base/<dl.Name>.torrent`.
  - Adjacent to payload: `<file>.aria2` next to files that came from `aria2.getFiles` or `dl.Files` for this download.
  - Trimmed leading-tags: `<trimmed>.aria2`/`.torrent` only if we can strongly prove ownership (existing matching sidecar or at least two basename matches found under the candidate folder).
- Deduplication: duplicate delete candidates are removed to avoid repeated log lines and filesystem calls.
- Directory pruning: empty parent directories are pruned deepest-first. A best‑effort removal of a root `.aria2` is attempted only when that root is proven to belong to the download.
- Symlinks: if a payload path is a symlink, the link itself is removed; the symlink target is not touched.

## Configuration

Common environment variables:
- `TORRUS_API_TOKEN` – required for protected endpoints
- `TORRUS_CLIENT` – `aria2` to enable the aria2 adapter (default: noop)
- `ARIA2_RPC_URL`, `ARIA2_SECRET`, `ARIA2_POLL_MS` – aria2 config
- `LOG_FORMAT` (`text|json`), `LOG_FILE_PATH`, `LOG_MAX_SIZE`, `LOG_MAX_BACKUPS`, `LOG_MAX_AGE_DAYS`

### Images

- Dev image (branch `dev`): built from Dockerfile `debug` stage.
- Release images (tags `v*` and `latest`): built from `prod` stage (distroless, non-root).

### Postgres storage (Kubernetes)

Enable Postgres-backed storage with:
- `TORRUS_STORAGE=postgres`
- Connection envs (defaults in parentheses):
  - `POSTGRES_HOST` (postgres)
  - `POSTGRES_PORT` (5432)
  - `APP_DB` (torrus)
  - `APP_USER` (torrus)
  - `APP_PASSWORD` (no default; from Secret)
  - `POSTGRES_SSLMODE` (disable)

Example Secret:

```
apiVersion: v1
kind: Secret
metadata:
  name: postgres-auth
type: Opaque
stringData:
  POSTGRES_PASSWORD: "changeMeAdmin"
  APP_DB: "torrus"
  APP_USER: "torrus"
  APP_PASSWORD: "changeMeApp"
```

Deployment envs:

```
- name: TORRUS_STORAGE
  value: postgres
- name: POSTGRES_HOST
  value: postgres
- name: POSTGRES_PORT
  value: "5432"
- name: APP_DB
  valueFrom:
    secretKeyRef:
      name: postgres-auth
      key: APP_DB
- name: APP_USER
  valueFrom:
    secretKeyRef:
      name: postgres-auth
      key: APP_USER
- name: APP_PASSWORD
  valueFrom:
    secretKeyRef:
      name: postgres-auth
      key: APP_PASSWORD
```

Notes:
- For MVP the service auto-creates a `downloads` table (UNIQUE `fingerprint`). Future migrations will replace this.
- The Postgres connection closes cleanly on shutdown.
- Defaults remain in-memory when `TORRUS_STORAGE` is unset.

## Deployment Examples

### Minimal Docker (debug)
```
docker run -p 9090:9090 \
  -e TORRUS_API_TOKEN=dev-token \
  -e TORRUS_CLIENT=aria2 \
  -e ARIA2_RPC_URL=http://host.docker.internal:6800/jsonrpc \
  ghcr.io/tinoosan/torrus:dev
```

### Kubernetes (prod image)
See docs for more: docs/running.md and docs/operations.md. A minimal env block:
```
env:
- name: TORRUS_STORAGE
  value: postgres
- name: TORRUS_API_TOKEN
  valueFrom:
    secretKeyRef: { name: api-auth, key: TOKEN }
# Postgres envs as shown above
```

## Endpoints (v1)

### Downloads

**GET /v1/downloads**  
Returns `200 OK` with a JSON array of [Download](#download-object) resources.

**GET /v1/downloads/{id}**  
Path parameters:
- `id` — numeric identifier  
Returns `200 OK` with a single [Download](#download-object). Responds with `404` if the ID is not found.

**POST /v1/downloads**  
Create a download (idempotent).  
Request body:
```json
{
  "source": "magnet:?xt=urn:btih:...",
  "targetPath": "/downloads/",
  "desiredStatus": "Active" // optional, defaults to "Queued"
}
```
Responds with:
- `201 Created` with the created [Download](#download-object) on the first request for a given `(source, targetPath)` pair.
- `200 OK` with the existing [Download](#download-object) for subsequent identical requests (idempotent POST).

Idempotency details:
- Torrus computes a stable fingerprint `sha256(normalize(source), normalize(targetPath))` where `normalize` trims whitespace and cleans the target path (via `filepath.Clean`).
- On Unix, paths remain case-sensitive. A Windows-specific normalization (e.g., lowercasing) can be added later if needed.

**PATCH /v1/downloads/{id}**
Update the desired status of a download.
Path parameters:
- `id` — numeric identifier
Request body:
```json
{ "desiredStatus": "Active|Resume|Paused|Cancelled" }
```
Responds with `200 OK` and the updated [Download](#download-object).
May return `409 Conflict` when a conflicting file already exists at the target.

**DELETE /v1/downloads/{id}**
Delete a download. Optional JSON body:
```json
{ "deleteFiles": true }
```
- `deleteFiles=true` removes on-disk files and control artifacts before deleting the entry.
- `deleteFiles=false` (default) only cancels and deletes the entry.
Returns `204 No Content` on success. Responds with `404` if the ID is not found.

#### Download Object
Fields returned by the downloads API:

| Field           | Type   | Description                                                                 |
|-----------------|--------|-----------------------------------------------------------------------------|
| `id`            | int    | Unique identifier (read-only)                                               |
| `gid`           | string | Backend identifier, may be `null` (read-only)                               |
| `source`        | string | Download source link (magnet URI, HTTP URL, etc.)                           |
| `targetPath`    | string | Destination path for the download                                           |
| `name`          | string | Human-friendly display name from the downloader (read-only, optional)       |
| `files`         | array  | Read-only list of file entries (when available). Each file has `path`, optional `length` and `completed`. |
| `status`        | string | Current status. One of `Queued`, `Active`, `Paused`, `Complete`, `Cancelled`, `Failed` (read-only) |
| `desiredStatus` | string | Desired status. Same enum as `status`                                       |
| `createdAt`     | string | RFC3339 timestamp when the download was created (read-only)                 |

### Health & Metrics

- `GET /healthz` (liveness): always returns `200 OK` with body `ok`.
- `GET /readyz` (readiness): returns `200 OK` when the active downloader is ready.
  - When using aria2, Torrus performs a fast JSON‑RPC probe.
  - When using the noop downloader, readiness returns `200 OK`.
- `GET /metrics`: Prometheus metrics in the standard exposition format.

Example Kubernetes probes:

```
livenessProbe:
  httpGet: { path: /healthz, port: 9090 }
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet: { path: /readyz, port: 9090 }
  initialDelaySeconds: 2
  periodSeconds: 5
```

Prometheus scrape example:

```
scrape_configs:
  - job_name: torrus
    static_configs:
      - targets: ['torrus:9090']
```

## Example Requests

```bash
# List downloads
curl http://localhost:8080/v1/downloads

# Create a new download
curl -X POST http://localhost:8080/v1/downloads \
  -H "Content-Type: application/json" \
  -d '{"source":"magnet:?xt=urn:btih:...","targetPath":"/downloads","desiredStatus":"Active"}'

# Get a download by ID
curl http://localhost:8080/v1/downloads/2a1f8d7e-3b4c-4d5e-8f9a-1b2c3d4e5f60

# Update download desired status
curl -X PATCH http://localhost:8080/v1/downloads/2a1f8d7e-3b4c-4d5e-8f9a-1b2c3d4e5f60 \
  -H "Content-Type: application/json" \
  -d '{"desiredStatus":"Paused"}'

# Health check
curl http://localhost:8080/healthz
```

## Contributing & Architecture
See [docs/README.md](docs/README.md) for developer guides and architecture details.

# Torrus API

Torrus is a work-in-progress download orchestration microservice for managing and monitoring downloads. It abstracts tools like aria2 behind a REST API so media servers and other applications can request torrents, magnet links, or direct downloads without handling the downloader directly.

## Status
🚧 **Under active development** — APIs, data models, and behavior are subject to change.

## Features (Planned / In Progress)
- Create and list downloads
- Update download state
- Retrieve download details

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

## API Overview

Torrus exposes a JSON-over-HTTP interface for orchestrating downloads.

- **Authentication** – Every request (except `/healthz`) must include an `Authorization: Bearer <token>` header. The token is supplied to the server via environment variable and unauthenticated calls return `401` or `403`.
- **Content negotiation** – Requests must set `Content-Type: application/json`. Bodies larger than ~1 MiB or containing unknown fields are rejected with `400`.
- **Logging** – Each call is logged with method, path, status code, duration, and bytes transferred to aid in debugging and monitoring.
- **Pluggable downloader** – The API delegates download work to a backend component. A no-op backend is used by default, but alternate downloaders (e.g., Aria2) can be enabled through configuration.
- **Port** – The service listens on port `9090` by default.

## Configuration

Environment variables:
- `TORRUS_CLIENT`: Selects downloader backend (`aria2` to enable aria2 adapter; defaults to noop).
- `ARIA2_RPC_URL`, `ARIA2_SECRET`, `ARIA2_POLL_MS`: Configure the aria2 client and polling interval.

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

### Health

**GET /healthz**  
Unversioned endpoint returning `200 OK` and the text `ok`.

## Example Requests

```bash
# List downloads
curl http://localhost:8080/v1/downloads

# Create a new download
curl -X POST http://localhost:8080/v1/downloads \
  -H "Content-Type: application/json" \
  -d '{"source":"magnet:?xt=urn:btih:...","targetPath":"/downloads","desiredStatus":"Active"}'

# Get a download by ID
curl http://localhost:8080/v1/downloads/123

# Update download desired status
curl -X PATCH http://localhost:8080/v1/downloads/123 \
  -H "Content-Type: application/json" \
  -d '{"desiredStatus":"Paused"}'

# Health check
curl http://localhost:8080/healthz
```

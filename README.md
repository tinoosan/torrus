# Torrus API

Torrus is a work-in-progress API for managing and monitoring downloads.

## Status
🚧 **Under active development** — APIs, data models, and behavior are subject to change.

## Features (Planned / In Progress)
- Create and list downloads
- Update download state
- Retrieve download details

## Versioning Policy
All API endpoints are explicitly versioned starting with **v1**.  
Future breaking changes will be introduced under a new version (e.g., `/v2/...`).  
Unversioned endpoints are disabled by default to encourage consistent usage.

- Current version: **v1**
- Unversioned routes: return `404` (or may optionally redirect to `/v1/...`)
- Futre health check endpoints (e.g., `/healthz`) will remain unversioned

## Endpoints (v1)

### Downloads
- `GET    /v1/downloads` → List downloads
- `GET    /v1/downloads/{id}` → Retrieve a single download
- `POST   /v1/downloads` → Create a new download
- `PATCH  /v1/downloads/{id}` → Update download state

### Health
- `GET    /healthz` → Service health check (unversioned)

## Example Requests

```bash
# List downloads
curl http://localhost:8080/v1/downloads

# Get a download by ID
curl http://localhost:8080/v1/downloads/123

# Create a new download
curl -X POST http://localhost:8080/v1/downloads \
  -H "Content-Type: application/json" \
  -d '{"url": "magnet:?xt=urn:btih:..."}'

# Update download state
curl -X PATCH http://localhost:8080/v1/downloads/123 \
  -H "Content-Type: application/json" \
  -d '{"status": "paused"}'

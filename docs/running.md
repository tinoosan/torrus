# Running Torrus

## Locally with Go
```bash
go run ./cmd
```

## Docker
```bash
docker build -t torrus .
docker run -p 9090:9090 torrus
```

## Docker Compose (Torrus + aria2)
```bash
docker compose up --build
```
This uses the provided `docker-compose.yaml` to run Torrus alongside aria2.

## Environment Variables
- `TORRUS_CLIENT` – set to `aria2` to enable the aria2 adapter.
- `ARIA2_RPC_URL`, `ARIA2_SECRET`, `ARIA2_POLL_MS` – aria2 client config.
- `LOG_FORMAT` (`text`|`json`), `LOG_FILE_PATH`, `LOG_MAX_SIZE`, `LOG_MAX_BACKUPS`, `LOG_MAX_AGE_DAYS`.

## Example Requests
```bash
# Create a download
curl -X POST http://localhost:9090/v1/downloads \
  -H 'Content-Type: application/json' \
  -d '{"source":"magnet:?xt=urn:btih:...","targetPath":"/downloads","desiredStatus":"Active"}'

# List downloads
curl http://localhost:9090/v1/downloads

# Resume a paused download
curl -X PATCH http://localhost:9090/v1/downloads/1 \
  -H 'Content-Type: application/json' \
  -d '{"desiredStatus":"Resume"}'

# Delete and remove files
curl -X DELETE http://localhost:9090/v1/downloads/1 \
  -H 'Content-Type: application/json' \
  -d '{"deleteFiles":true}'
```

Smoke-test the health endpoint:
```bash
curl http://localhost:9090/healthz
```

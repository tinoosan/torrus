# Running Locally

## Who this is for
Developers spinning up Torrus for manual testing.

## What you'll learn
How to run the stack with Docker Compose, issue requests and troubleshoot common issues.

### Start services
Requires Docker with Compose v2.
```
docker compose up --build
```
This launches Torrus and aria2; environment can be tweaked via `.env`
(see [configuration](configuration.md)). API listens on `localhost:9090`.

### Example requests
```bash
# Create download
curl -X POST http://localhost:9090/v1/downloads \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer local-token' \
  -d '{"source":"https://speed.hetzner.de/1MB.bin","targetPath":"/data"}'

# Resume a download
curl -X PATCH http://localhost:9090/v1/downloads/1 \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer local-token' \
  -d '{"desiredStatus":"Resume"}'
```
Postman collections can import the OpenAPI spec [`index.yaml`](../index.yaml).

### Test links
- HTTP: `https://speed.hetzner.de/1MB.bin`
- BitTorrent: `magnet:?xt=urn:btih:d2474e86c95b19b8bcfdb92bc12c9d44667cfa36`

### Troubleshooting
- Port 9090 busy → set `-p 8080:9090` on the compose service.
- TLS handshake errors → aria2 not ready; wait for its logs to show `RPC: listening`.
- Duplicate `.aria2` files → disable partial file writing or clean target dir.

More workflows are in [ci-cd](ci-cd.md).

# Observability: Health, Readiness, and Metrics

Torrus exposes unversioned health and readiness endpoints for container probes,
and a Prometheus `/metrics` endpoint for scraping runtime metrics.

## Endpoints

- `GET /healthz` — Liveness probe. Always returns `200 OK` with body `ok` if the process is running.
- `GET /readyz` — Readiness probe. Returns:
  - `200 OK` with `{ "ready": true }` when the active downloader is ready.
  - `503 Service Unavailable` with `{ "ready": false, "error": "..." }` when checks fail.
- `GET /metrics` — Prometheus exposition format.

Authentication bypass: these three endpoints do not require the `Authorization` header.

## Readiness Details

- When the active downloader is `aria2`, Torrus performs a fast JSON‑RPC call (e.g., `aria2.getVersion`) with a ~300ms timeout.
- If the adapter supports a `Ping(context.Context) error` method and it returns an error, readiness responds 503.
- The `noop` downloader is always considered ready.

## Prometheus Metrics

Collectors registered under the `torrus_` namespace:

- `torrus_download_events_total{type}` (counter): Reconciler event counts. Types include `start|progress|paused|cancelled|complete|failed|meta|gid_update`.
- `torrus_aria2_rpc_errors_total{method}` (counter): aria2 JSON‑RPC error counts per method.
- `torrus_aria2_rpc_latency_seconds{method}` (histogram): aria2 JSON‑RPC latency per method.
- `torrus_active_downloads` (gauge): Number of active GIDs tracked by the aria2 adapter.

### Instrumentation Sources

- Reconciler increments `torrus_download_events_total` for each event handled.
- Aria2 adapter:
  - Wraps RPC calls to observe `torrus_aria2_rpc_latency_seconds{method}` and increments `torrus_aria2_rpc_errors_total{method}` on failures.
  - Updates `torrus_active_downloads` whenever the tracked active GIDs set changes.

## Kubernetes Probes

Example `Deployment` snippet:

```yaml
livenessProbe:
  httpGet: { path: /healthz, port: 9090 }
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet: { path: /readyz, port: 9090 }
  initialDelaySeconds: 2
  periodSeconds: 5
```

## Prometheus Scrape

Example static scrape config:

```yaml
scrape_configs:
  - job_name: torrus
    static_configs:
      - targets: ["torrus:9090"]
```

## Local Verification

```bash
curl -i http://localhost:9090/healthz
curl -i http://localhost:9090/readyz
curl -s http://localhost:9090/metrics | head
```


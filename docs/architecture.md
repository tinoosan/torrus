# Architecture

## Who this is for
New contributors who need an end-to-end picture before touching the code.

## What you'll learn
Service boundaries, component responsibilities, concurrency patterns and
where the canonical API spec lives.

Torrus exposes a small HTTP API for orchestrating downloads. Handlers
delegate to a service layer, which drives a downloader and persists
state in an in-memory repository.

```
+---------+      +-----------+      +--------------+      +-----------------+
| Handlers| ---> |  Service  | ---> |  Downloader  | ---> |   External svc  |
|  (v1)   |      | (usecases)|      | (aria2, noop)|      | (aria2 JSON-RPC)|
+----+----+      +-----+-----+      +------+-------+      +---------+-------+
     |                  |                   |                        |
     v                  v                   v                        |
+----+------------------+-------------------+------------------------+------+
|                                Repo (in-mem)                              |
|                (Update mutate-fn, Reader/Writer ports)                    |
+----------------------------------------------------------------------------+
                             ^
                             |
                        +----+-----+
                        |Reconciler|
                        +----------+
```

> TODO: Mermaid version later

### Layer responsibilities
- **Handlers** – HTTP routing, auth middleware and JSON encoding.
- **Service** – business rules, idempotency and coordination of repo and
  downloader actions.
- **Repo** – in-memory persistence with `Update` mutation closures to
  ensure atomic writes.
- **Downloader** – pluggable adapter (`aria2`, `noop`) implementing
  `Start`, `Pause`, `Resume`, `Cancel`, `Delete`.
- **Reconciler** – consumes events from downloaders and updates the
  repository accordingly.

### Concurrency model
The repository guards its slice with a `sync.RWMutex` and executes
mutations while holding the lock. Downloaders emit events through a
`Reporter` channel; the reconciler listens on that channel and serially
applies updates via `Repo.Update`.

### OpenAPI
The canonical spec lives in [`index.yaml`](../index.yaml) (see
[openapi.md](openapi.md)). Fields such as `name` and `files[]` are
read-only in responses.

Continue with the [request flow](request-flow.md) and
[downloader + reconciler](downloader-and-reconciler.md) docs for deeper
dives.

## Versioning
All API routes are under `/v1`. The health check `/healthz` is left
unversioned for infrastructure probes.

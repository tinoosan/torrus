# Logging and Correlation

This document explains how Torrus correlates logs across HTTP requests and long-running components.

## Request IDs (`X-Request-ID`)

- The API accepts an optional `X-Request-ID` header on every request.
- A middleware reads it (or generates a UUIDv4 when missing), stores it in the request `context.Context`, and echoes it in the response header.
- Handlers, services, repos, and downloaders can enrich their logs by extracting the ID via `reqid.From(ctx)` and logging it as `request_id`.
- Background goroutines spawned during a request (e.g., initial start flow) copy the `request_id` into a new background context so subsequent logs from the downloader include the same ID.

Example

```
curl -H 'X-Request-ID: my-debug-id' http://localhost:9090/v1/downloads -i
```

## Operation IDs for Long-Running Components

Components not triggered by HTTP (no incoming request context) attach a one-time `operation_id` at startup:

- Reconciler run loop.
- Aria2 adapter poller/notification loop.

This adds a stable field to logs that helps correlate activity across time without inventing a `request_id`.

## Best Practices

- Do not thread `request_id` manually in function parameters; always read it from `context.Context`.
- When spawning goroutines in a request, create a new `context.Background()` and copy the `request_id` only (avoid leaking timeouts/cancels you don’t control).
- Prefer structured logging with `slog` and use stable keys: `request_id`, `operation_id`.

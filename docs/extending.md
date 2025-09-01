# Extending Torrus

## Who this is for
Engineers adding features or integrating new download backends.

## What you'll learn
Where to plug in new downloaders, handle fresh events and update the API spec.

### Add a downloader
1. Implement the [`Downloader`](packages.md#internaldownloader) interface.
2. Emit events via a `Reporter` and optionally implement `EventSource`.
3. Wire the adapter in `cmd/main.go` behind `TORRUS_CLIENT`.
4. Avoid touching handlers or the repo; the service and reconciler drive state.

### New event types
- Extend `downloader.Event` with the extra fields.
- Update the [reconciler](downloader-and-reconciler.md) to persist them.
- Document how the field surfaces in responses.

### OpenAPI checklist
- Edit [`index.yaml`](../index.yaml) to describe new routes or fields.
- Mark read-only fields like `name` or `files[]` appropriately.
- Regenerate or update examples and mention changes in [request flow](request-flow.md).

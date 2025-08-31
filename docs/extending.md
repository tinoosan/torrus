# Extending Torrus

## Add a Downloader Adapter
1. Implement the [`Downloader`](packages.md#internaldownloader) interface.
2. Emit events through a `Reporter` and optionally expose `Run(ctx)` to
   satisfy `EventSource`.
3. Wire the adapter in `cmd/main.go` based on an environment switch.
4. Implement a `Purge` method for deleting on-disk files.

## Add Fields via Events
- Extend `downloader.Event` with the new metadata.
- Update the reconciler to persist the field.
- Document the field in the API and OpenAPI spec.

## New Service Use‑Cases
- Add methods to `internal/service` that express the use‑case.
- Keep handlers thin: call the service and translate errors.
- Avoid leaking repository or downloader details across layers by
  introducing narrow interfaces where needed.

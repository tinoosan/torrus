# Persistence and Repo

## Who this is for
Anyone touching storage logic or reasoning about state transitions.

## What you'll learn
How the in-memory repo works, mutation patterns, and the status model.

### Repository interfaces
`DownloadReader`/`DownloadWriter` provide basic CRUD. `Repo.Update(ctx,
id, func(*Download) error)` applies a mutation while holding a lock,
returning a deep clone of the updated entity.

### Deep-copy safety
`Download.Clone()` prevents callers from accidentally mutating stored
state. Always work on the clone returned by repo methods.

### Status model
- `status` – last known state from the downloader.
- `desiredStatus` – caller intent.
- Terminal states: `Cancelled`, `Complete`, `Failed`.
- GID is assigned on first `Start` and cleared on `Cancelled` or purge.

### ID & timestamps
IDs auto-increment inside the repo (`nextID`). `CreatedAt` defaults in the
service when zero so tests can omit it safely.

See [idempotency](idempotency.md) for fingerprinting logic and
[events and states](events-and-states.md) for the full lifecycle table.

# Packages

Short summaries of key packages and their extension points.

## api/v1
- HTTP handlers and middleware.
- Logging and auth via thin wrappers.
- Validates requests and delegates to the service.

## internal/service
- Implements the download service contract.
- Orchestrates repository updates and downloader actions.
- Uses small interfaces for dependency injection.

## internal/repo
- Defines `DownloadReader`, `DownloadWriter`, `DownloadFinder`.
- `Update` accepts a mutation closure for atomic changes.
- `inmem` provides an in-memory implementation.

## internal/downloader
- Core `Downloader` interface (`Start`, `Pause`, `Resume`, `Cancel`, `Delete`).
- `Event` model and `Reporter` channel helper.
- Noop adapter for testing and aria2 adapter under `downloader/aria2`.

## internal/reconciler
- Consumes downloader events.
- Updates repository state and handles GID semantics.
- Ensures terminal events match the last known GID.

## internal/aria2
- JSON‑RPC client built from environment variables.
- Used by the aria2 downloader adapter.

## cmd/
- Main wiring: flag/env parsing, logging setup, repo/service wiring.
- Selects downloader backend via `TORRUS_CLIENT`.

# Operations: Correlation & Deletion Safety

This page consolidates guidance for log correlation and safe deletion behavior.

## Correlation IDs (X-Request-ID)

- Middleware accepts optional `X-Request-ID`; generates UUIDv4 when absent.
- The value is stored in `context.Context` and echoed in the response header.
- Handlers/services/repos/adapters enrich logs with `request_id` obtained via `reqid.From(ctx)`.
- Background work spawned by a request copies only the `request_id` into a fresh `context.Background()`.

Example

```
curl -H 'X-Request-ID: my-debug-id' http://localhost:9090/v1/downloads -i
```

## Operation IDs (long-running)

- Components without an incoming request (Reconciler, aria2 poller) attach a one-time `operation_id` at startup for correlation.

## Deletion Semantics & Safety

- Never delete the base `targetPath` itself; only paths strictly under `targetPath/` are eligible.
- Normalize and validate all candidate paths; reject anything outside `targetPath/`.
- Deduplicate candidates to avoid duplicate filesystem calls and log lines.
- Prune empty parent directories deepest-first after payload removal.
- Best‑effort root `.aria2` removal only if the pruned root is proven to belong to the download.
- Symlinks: remove the symlink itself; never act on the target.

### Sidecar Ownership Rules

Sidecars are `.aria2` and `.torrent`. Remove them only when ownership is proven:

1) Exact name match
- `base/<dl.Name>.aria2` and (for torrents) `base/<dl.Name>.torrent`.
- If the logical root equals `base/<dl.Name>`, then also `root.aria2` / `root.torrent`.

2) Adjacent to known payload
- `<file>.aria2` where `<file>` is a payload path from `aria2.getFiles` or `dl.Files` for the download.

3) Trimmed leading‑tags (best‑effort)
- For names like `[TAG] Show.S01`, allow `<trimmed>.aria2`/`.torrent` only if:
  - Such a sidecar already exists, or
  - At least two distinct basenames in `dl.Files` appear under the candidate folder.

If no rule matches, leave sidecars untouched.

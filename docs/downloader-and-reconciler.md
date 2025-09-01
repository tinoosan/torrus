# Downloader and Reconciler

## Who this is for
Engineers implementing adapters or tracing event handling.

## What you'll learn
The downloader interface, aria2 specifics, reporter events and how the
reconciler mutates repository state.

### Downloader contract
Downloaders expose `Start`, `Pause`, `Cancel`, `Resume` and `Delete`. They
operate on immutable snapshots of `Download` objects. Adapters may also
satisfy `EventSource` and emit events through a `Reporter` channel.

### aria2 adapter
- `Start` → `aria2.addUri`
- `Resume` → `aria2.unpause`
- `Pause`  → `aria2.pause`
- `Cancel` → `aria2.forceRemove`
- `Delete`  → `aria2.removeDownloadResult`
- Polling (`ARIA2_POLL_MS`) fills in progress if notifications are silent.

Logging & correlation:
- If a `request_id` exists in the incoming context, adapter logs include it.
- Long-running poll/notification loops add a stable `operation_id` at startup.

Deletion safety:
- See [Operations: Correlation & Deletion Safety](operations.md) for ownership-based sidecar removal and path safety guarantees.

### Reporter events
`Start`, `Paused`, `Cancelled`, `Complete`, `Failed`, `Progress`, `Meta`
and `GIDUpdate`. Metadata events supply resolved `name` and `files[]`.

### Reconciler rules
- Listens on the reporter channel.
- Ignores events whose `gid` does not match the repo snapshot.
- Only mutates via `Repo.Update`.
- Swaps `gid` on `GIDUpdate` and stores file metadata from `Meta` events.

```
aria2 ---> Adapter --emit--> Reporter --chan--> Reconciler --Update--> Repo
```

> TODO: Mermaid version later

See [persistence and repo](persistence-and-repo.md) for mutation safety
and [request flow](request-flow.md) for how events fit into API calls.

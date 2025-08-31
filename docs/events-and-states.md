# Events and States

Downloads move through several statuses and emit events as backends progress.

## Lifecycle
`Queued → Active → Paused → Cancelled | Complete | Failed`

`Resume` is a desired status used to transition a paused download back to `Active`.

## Desired vs Actual Status
- `desiredStatus` reflects what the client asked for.
- `status` shows the last known state from the downloader.
- The reconciler brings `status` in line with `desiredStatus` when events arrive.

## Events
Downloaders publish events through a `Reporter` channel:

| Event | Purpose |
|-------|---------|
| `Start` | Download began; repo status becomes `Active`. |
| `Paused` | Downloader paused the transfer. |
| `Cancelled` | Transfer cancelled; clears `gid`. |
| `Complete` | Transfer finished successfully. |
| `Failed` | Terminal error; status `Failed`. |
| `Progress` | Optional progress metrics (bytes, speed). |
| `Meta` | Metadata such as resolved `name` and `files`. |
| `GIDUpdate` | Swap to a new backend identifier. |

## GID Semantics
- Each download may have a backend `gid`.
- Terminal events (`Cancelled`, `Complete`, `Failed`) are applied only if the
  event's `gid` matches the repo's current `gid`.
- `GIDUpdate` events replace the stored `gid` when downloaders swap IDs.

## Metadata Updates
`Meta` events can populate read‑only fields like `name` and `files`.
The reconciler overwrites the current snapshot with whatever the adapter reports.

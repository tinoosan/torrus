# Glossary

## Who this is for
Readers encountering Torrus-specific terminology.

## What you'll learn
Definitions of common terms.

| Term | Meaning |
|------|---------|
| **GID** | Backend identifier returned by a downloader like aria2. |
| **desiredStatus** | Client's requested state (`Active`, `Paused`, `Resume`, `Cancelled`). |
| **status** | Last known state from the downloader. |
| **terminal state** | `Cancelled`, `Complete` or `Failed`—no further transitions expected. |
| **Reporter** | Interface used by downloaders to emit events to the reconciler. |

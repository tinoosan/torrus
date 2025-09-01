# Idempotency

## Who this is for
Clients and backend engineers ensuring safe retries.

## What you'll learn
How Torrus deduplicates `POST /v1/downloads` requests.

### POST semantics
The service computes a SHA-256 fingerprint over normalized `source` and
`targetPath` ([details](persistence-and-repo.md)). If a download with the
same fingerprint exists, the repo returns it and the handler responds with
`200 OK` instead of creating a new row.

### Idempotency-Key header
If clients send `Idempotency-Key`, the handler still uses the
fingerprint—duplicates are detected even without the header. Future
adapters may use the header to partition keys.

### What counts as "same request"
Identical normalized `source` + `targetPath`. Different destinations or
sources (even if they resolve to the same file) are treated as new.

### Responses
- `201 Created` – new download started.
- `200 OK` – duplicate suppressed; existing ID returned.

Caveat: changing the payload after a retry (e.g., `desiredStatus`) still
returns the first request's values. Resubmit a new download if intent
changed.

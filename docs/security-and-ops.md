# Security & Ops (Stub)

## Authentication
Pluggable authentication is planned but not yet implemented.

## Idempotency
POST `/v1/downloads` is idempotent based on the source and target path
fingerprint. Repeating the same request returns the existing download.

## Logging
Logs can be written in text or JSON (`LOG_FORMAT`) and rotate via
`LOG_FILE_PATH` and related env vars.

## Future
- Metrics and structured traces.
- Webhook notifications in a future `v2` API.

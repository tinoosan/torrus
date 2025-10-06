# Changelog

All notable changes to this project will be documented in this file.

## 0.1.0 – 2025-09-20

- Storage: Add PostgreSQL-backed repository (opt-in via `TORRUS_STORAGE=postgres`).
  - DSN from env via `POSTGRES_DB_URL` (e.g., `postgres://user:pass@host:5432/db?sslmode=disable`).
  - Auto-creates `downloads` table with UNIQUE `fingerprint` for MVP.
  - Graceful shutdown: close DB on server exit.
  - Update is transactional with `SELECT FOR UPDATE` to prevent lost updates.
  - Preserve `createdAt` (immutable) on updates.
- Downloader: Split aria2 adapter into focused files (rpc, ops, files, delete, events) with no behavior change.
- API: DRY strict JSON decode in v1 middlewares (content-type, size limit, unknown fields).
- Observability: Retain Prometheus metrics and structured logging; add minor adapter gauges/counters.
- Docs: Add Postgres/K8s configuration snippet to README.

Notes
- Default storage remains in-memory unless `TORRUS_STORAGE=postgres` is set.
- Future work (tracked as issues): CORS allowlist, rate limiting, Postgres integration tests, migrations tooling, downloader shutdown wiring, CI staticcheck & Go matrix, Makefile, and adapter module docs.

# Configuration

## Who this is for
Operators and developers configuring Torrus.

## What you'll learn
Environment variables, defaults and a sample `.env`.

### Environment variables
| Variable | Default | Purpose |
|----------|---------|---------|
| `TORRUS_CLIENT` | `noop` | Downloader adapter (`aria2` enables the aria2 client). |
| `TORRUS_API_TOKEN` | *(required)* | Bearer token for API auth. |
| `ARIA2_RPC_URL` | `http://127.0.0.1:6800/jsonrpc` | aria2 JSON-RPC endpoint. |
| `ARIA2_SECRET` | empty | aria2 RPC secret. |
| `ARIA2_TIMEOUT_MS` | `3000` | HTTP timeout for aria2 client. |
| `ARIA2_POLL_MS` | `1000` | Polling interval for progress. |
| `LOG_FORMAT` | `text` | Log format: `text` or `json`. |
| `LOG_FILE_PATH` | `./logs/torrus.log` | Log output path (dir auto-created). |
| `LOG_MAX_SIZE` | `1` | Rotate after N MB. |
| `LOG_MAX_BACKUPS` | `3` | Number of rotated files to keep. |
| `LOG_MAX_AGE_DAYS` | `7` | Days to retain logs. |

#### Storage (Postgres)
| Variable | Default | Purpose |
|----------|---------|---------|
| `TORRUS_STORAGE` | empty | Set to `postgres` to enable Postgres-backed repo (otherwise in-memory). |
| `POSTGRES_DB_URL` | empty | Full Postgres DSN, e.g. `postgres://user:pass@host:5432/db?sslmode=disable`. |

### Example `.env`
```
TORRUS_CLIENT=aria2
TORRUS_API_TOKEN=local-token

ARIA2_RPC_URL=http://localhost:6800/jsonrpc
ARIA2_SECRET=changeme
ARIA2_TIMEOUT_MS=5000
ARIA2_POLL_MS=1000

LOG_FORMAT=json
LOG_FILE_PATH=./logs/torrus.log
LOG_MAX_SIZE=5
LOG_MAX_BACKUPS=2
LOG_MAX_AGE_DAYS=7

# Storage (opt-in Postgres)
TORRUS_STORAGE=postgres
POSTGRES_DB_URL=postgres://torrus:changeMeApp@postgres:5432/torrus?sslmode=disable
```

See [running locally](running-locally.md) for using this file with
`docker-compose`.

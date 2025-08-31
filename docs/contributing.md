# Contributing

## Branching
- Work on a feature branch.
- Open a PR into `dev`.
- `dev` merges to `staging` for release candidates and then to `main`.

## Pull Requests
- Use the PR template and describe the change.
- Follow [Conventional Commits](https://www.conventionalcommits.org/)
  (e.g. `feat:`, `fix:`, `docs:`).

## CI
- `lint`: static checks and formatting.
- `ci`: unit tests.
- `integration`: docker compose end‑to‑end tests.
- `docs-links`: verifies markdown links under `docs/`.

## Testing Guidance
- Run unit tests: `go test ./...`.
- Use `go test -race` for race detection when touching concurrent code.
- Handler tests live alongside handlers; adapter tests can mock JSON‑RPC calls.
- Aim for high coverage but focus on critical paths.

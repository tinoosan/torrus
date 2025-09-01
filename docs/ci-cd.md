# CI/CD

## Who this is for
Contributors verifying changes before opening a PR.

## What you'll learn
The GitHub workflows and how to run the same checks locally.

### Workflows
- **lint.yaml** – runs `golangci-lint`.
- **ci.yaml** – verifies module graph, builds, vets and runs unit tests with
  race detector and coverage.
- **integration.yaml** – builds a Docker image and exercises the API via
  `docker compose`.
- **docker-build.yaml** – builds release images on `main`.

### Run locally
```bash
# Lint
golangci-lint run

# Unit tests
go test -race ./...

# Integration (aria2 + api)
docker compose up --build

# Image build
docker build -t torrus .
```

See [running locally](running-locally.md) for environment expectations.

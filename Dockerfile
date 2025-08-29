FROM golang:1.24-alpine AS builder

WORKDIR /src

# Faster, reproducible builds
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto

# System deps
RUN apk add --no-cache ca-certificates

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the app (strip debug symbols to shrink)
RUN go build -ldflags="-s -w" -o /out/torrus ./cmd

# Prepare a log directory to copy into the final image
RUN mkdir -p /out/var/log/torrus


FROM gcr.io/distroless/static:nonroot

# Set working directory (optional)
WORKDIR /

# Copy binary and log dir from builder
COPY --from=builder --chown=nonroot:nonroot /out/torrus /torrus
COPY --from=builder --chown=nonroot:nonroot /out/var/log/torrus /var/log/torrus

# Default envs (override at runtime as needed)
# LOG_FORMAT: text|json
ENV LOG_FORMAT=json
ENV LOG_FILE_PATH=/var/log/torrus/torrus.log
ENV LOG_MAX_SIZE=10     
ENV LOG_MAX_BACKUPS=3
ENV LOG_MAX_AGE_DAYS=7

# Distroless already runs as nonroot user
USER nonroot:nonroot

# The API listens on 9090 by default
EXPOSE 9090

ENTRYPOINT ["/torrus"]

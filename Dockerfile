# ---------- builder ----------
FROM golang:1.24-alpine AS builder

WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto

# System deps for TLS roots
RUN apk add --no-cache ca-certificates

# Cache modules
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN go build -ldflags="-s -w" -o /out/torrus ./cmd

RUN mkdir -p /out/var/log/torrus
RUN cp /etc/ssl/certs/ca-certificates.crt /out/ca-certificates.crt


# ---------- prod ----------
FROM gcr.io/distroless/static:nonroot AS prod
WORKDIR /

COPY --from=builder --chown=nonroot:nonroot /out/torrus /torrus
COPY --from=builder --chown=nonroot:nonroot /out/var/log/torrus /var/log/torrus
COPY --from=builder --chown=nonroot:nonroot /out/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

ENV LOG_FORMAT=json
ENV LOG_FILE_PATH=/var/log/torrus/torrus.log
ENV LOG_MAX_SIZE=10
ENV LOG_MAX_BACKUPS=3
ENV LOG_MAX_AGE_DAYS=7

USER nonroot:nonroot
EXPOSE 9090
ENTRYPOINT ["/torrus"]


# ---------- debug  ----------
FROM alpine:3.20 AS debug
WORKDIR /

RUN apk add --no-cache bash ca-certificates \
  && adduser -D -u 65532 appuser

# Ensure files and logs are owned by nonroot user
COPY --from=builder --chown=appuser:appuser /out/torrus /torrus
COPY --from=builder --chown=appuser:appuser /out/var/log/torrus /var/log/torrus
COPY --from=builder /out/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Carry logging defaults like prod so debug runs without overrides
ENV LOG_FORMAT=json
ENV LOG_FILE_PATH=/var/log/torrus/torrus.log
ENV LOG_MAX_SIZE=10
ENV LOG_MAX_BACKUPS=3
ENV LOG_MAX_AGE_DAYS=7

USER appuser

EXPOSE 9090
ENTRYPOINT ["/torrus"]

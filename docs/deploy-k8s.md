# Deploying Torrus on Kubernetes

This guide shows a minimal production-ready deployment using the distroless
`prod` image, a PostgreSQL StatefulSet, and environment variables wired from
Secrets. Adjust namespaces and resource values for your cluster.

## Prerequisites
- Kubernetes 1.25+
- A Postgres instance (example below assumes a Service `postgres`)
- A downloader (e.g., aria2 sidecar or external service)

## Secrets

Postgres credentials (you provided this layout):
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: postgres-auth
type: Opaque
stringData:
  POSTGRES_PASSWORD: "changeMeAdmin"
  APP_DB: "torrus"
  APP_USER: "torrus"
  APP_PASSWORD: "changeMeApp"
```

API token Secret:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: torrus-api-auth
type: Opaque
stringData:
  TOKEN: "changeMeAPI"
```

## Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: torrus-api
spec:
  selector:
    app: torrus-api
  ports:
  - name: http
    port: 9090
    targetPort: 9090
```

## Deployment (prod image)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: torrus-api
spec:
  replicas: 1
  selector: { matchLabels: { app: torrus-api } }
  template:
    metadata:
      labels: { app: torrus-api }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
      containers:
      - name: api
        image: ghcr.io/tinoosan/torrus:latest
        imagePullPolicy: IfNotPresent
        ports: [{ containerPort: 9090 }]
        env:
        - name: TORRUS_STORAGE
          value: postgres
        - name: TORRUS_API_TOKEN
          valueFrom: { secretKeyRef: { name: torrus-api-auth, key: TOKEN } }
        - name: TORRUS_CLIENT
          value: aria2
        - name: ARIA2_RPC_URL
          value: http://aria2:6800/jsonrpc
        # Postgres wiring
        - name: POSTGRES_HOST
          value: postgres
        - name: POSTGRES_PORT
          value: "5432"
        - name: APP_DB
          valueFrom: { secretKeyRef: { name: postgres-auth, key: APP_DB } }
        - name: APP_USER
          valueFrom: { secretKeyRef: { name: postgres-auth, key: APP_USER } }
        - name: APP_PASSWORD
          valueFrom: { secretKeyRef: { name: postgres-auth, key: APP_PASSWORD } }
        - name: POSTGRES_SSLMODE
          value: disable
        resources:
          requests: { cpu: 100m, memory: 128Mi }
          limits:   { cpu: 500m, memory: 512Mi }
        readinessProbe:
          httpGet: { path: /readyz, port: 9090 }
          initialDelaySeconds: 3
          periodSeconds: 5
        livenessProbe:
          httpGet: { path: /healthz, port: 9090 }
          initialDelaySeconds: 5
          periodSeconds: 10
        volumeMounts:
        - name: logs
          mountPath: /var/log/torrus
        # Distroless image runs as nonroot; no shell present.
      volumes:
      - name: logs
        emptyDir: {}
```

## Aria2 (optional)
You can run aria2 in the same namespace and reference it via `ARIA2_RPC_URL`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: aria2
spec:
  selector: { app: aria2 }
  ports: [{ port: 6800, targetPort: 6800 }]
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aria2
spec:
  replicas: 1
  selector: { matchLabels: { app: aria2 } }
  template:
    metadata:
      labels: { app: aria2 }
    spec:
      containers:
      - name: aria2
        image: p3terx/aria2-pro:latest
        ports: [{ containerPort: 6800 }]
        env:
        - name: RPC_SECRET
          value: ci-secret
```

## Notes
- The API uses a distroless image for releases; there is no shell. Use logs and health endpoints for troubleshooting.
- Set `TORRUS_API_TOKEN` in all environments. Health/readiness/metrics remain unauthenticated by design.
- For production, consider managed Postgres and set `POSTGRES_SSLMODE=require`.
- Future versions will use versioned DB migrations instead of auto-creating the schema.

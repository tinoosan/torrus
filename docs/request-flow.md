# Request Flow

## Who this is for
Developers tracing how API calls propagate through the system.

## What you'll learn
How POST and PATCH requests travel across handlers, services, the repo and the downloader.

### POST /v1/downloads
1. **Client → Handler** – JSON body is decoded and basic fields are validated.
2. **Handler → Service.Add** – passes the request struct and optional `Idempotency-Key` header.
3. **Service**
   - Fills defaults (`CreatedAt`, desired/status).
   - Computes a fingerprint from `source` + `targetPath` and calls `Repo.AddWithFingerprint`.
   - If the repo reports `created=true` and status is `Active`, it invokes `Downloader.Start`.
4. **Downloader** – returns a backend GID; service stores it via `Repo.Update`.
5. **Response** – 201 Created for new rows, 200 OK when an existing download matched the fingerprint.

### PATCH /v1/downloads/{id}
1. **Client → Handler** – desired status is parsed from JSON.
2. **Handler → Service.UpdateDesiredStatus** – validates the status and persists the `desiredStatus`.
3. **Service → Repo.Update** – stores intent, then performs side effects:
   - `Resume` or `Active`: `Downloader.Resume` or `Start`.
   - `Paused`: `Downloader.Pause`.
   - `Cancelled`: `Downloader.Cancel` and clears GID.
4. **Downloader → Reporter** – emits lifecycle events.
5. **Reconciler → Repo.Update** – brings actual `status` in line with events.

Example sequence for `PATCH` with `{"desiredStatus":"Resume"}`:
```
Client -> Handler: PATCH /v1/downloads/{id} {"desiredStatus":"Resume"}
Handler -> Service: UpdateDesiredStatus(id, Resume)
Service -> Repo: Update(id, mutate desiredStatus=Resume)
Service -> Downloader: Resume(dl) or Start(dl) if missing GID
Downloader -> Reporter: EventStart/EventPaused/EventCancelled/...
Reconciler -> Repo: Update(id, mutate status=Active|Paused|...)
```

See [idempotency](idempotency.md) for more on duplicate POST handling and
[downloader + reconciler](downloader-and-reconciler.md) for event details.

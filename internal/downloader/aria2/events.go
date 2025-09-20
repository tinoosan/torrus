package aria2dl

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "strconv"
    "time"

    "github.com/google/uuid"
    "github.com/tinoosan/torrus/internal/aria2"
    "github.com/tinoosan/torrus/internal/downloader"
    "github.com/tinoosan/torrus/internal/metrics"
)

// EmitComplete can be used by callers to signal that a download finished successfully.
func (a *Adapter) emitComplete(id string, gid string) {
    if a.rep != nil {
        a.rep.Report(downloader.Event{ID: id, GID: gid, Type: downloader.EventComplete})
    }
}

// EmitFailed signals that a download has failed.
func (a *Adapter) emitFailed(id string, gid string) {
    if a.rep != nil {
        a.rep.Report(downloader.Event{ID: id, GID: gid, Type: downloader.EventFailed})
    }
}

// EmitProgress publishes a progress update for the given download.
func (a *Adapter) emitProgress(id string, gid string, p downloader.Progress) {
    if a.rep != nil {
        a.rep.Report(downloader.Event{ID: id, GID: gid, Type: downloader.EventProgress, Progress: &p})
    }
}

// Run subscribes to aria2 notifications and emits corresponding downloader events.
func (a *Adapter) Run(ctx context.Context) {
    // Tag this run with a stable operation_id for correlation.
    opID := uuid.NewString()
    lg := a.log.With("operation_id", opID)
    ch, err := a.cl.Notifications(ctx)
    if err != nil {
        return
    }
    // Start poller goroutine for continuous progress updates
    go a.pollLoopWithLogger(ctx, lg)
    for {
        select {
        case <-ctx.Done():
            return
        case n, ok := <-ch:
            if !ok {
                return
            }
            a.handleNotification(ctx, n)
        }
    }
}

func (a *Adapter) handleNotification(ctx context.Context, n aria2.Notification) {
    for _, p := range n.Params {
        a.mu.RLock()
        id, ok := a.gidToID[p.GID]
        a.mu.RUnlock()
        if !ok {
            continue
        }
        switch n.Method {
        case "aria2.onDownloadComplete":
            // Before treating as terminal, check if this was a metadata task that spawned a real GID via followedBy.
            if ns, err := a.tellNameStatus(ctx, p.GID); err == nil && ns != nil && len(ns.FollowedBy) > 0 && ns.FollowedBy[0] != "" {
                real := ns.FollowedBy[0]
                a.mu.Lock()
                // carry over tracking to real gid
                delete(a.gidToID, p.GID)
                delete(a.activeGIDs, p.GID)
                idCopy := id
                a.gidToID[real] = idCopy
                a.activeGIDs[real] = struct{}{}
                if lp, ok := a.lastProg[p.GID]; ok {
                    a.lastProg[real] = lp
                    delete(a.lastProg, p.GID)
                }
                // update active downloads gauge
                metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
                a.mu.Unlock()
                if a.rep != nil {
                    a.rep.Report(downloader.Event{ID: id, GID: p.GID, Type: downloader.EventGIDUpdate, NewGID: real})
                }
                // Emit meta (name from ns or files) for the real gid
                var meta downloader.Meta
                if name := a.deriveName(ns, ""); name != "" { // source not known here
                    meta.Name = &name
                }
                if files := a.getFiles(ctx, real); files != nil {
                    meta.Files = &files
                }
                if meta.Name != nil || meta.Files != nil {
                    a.rep.Report(downloader.Event{ID: id, GID: real, Type: downloader.EventMeta, Meta: &meta})
                }
                // Do not emit Complete for metadata gid
                continue
            }
            a.emitComplete(id, p.GID)
            a.mu.Lock()
            delete(a.gidToID, p.GID)
            delete(a.activeGIDs, p.GID)
            delete(a.lastProg, p.GID)
            // update active downloads gauge
            metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
            a.mu.Unlock()
        case "aria2.onDownloadError":
            a.emitFailed(id, p.GID)
            a.mu.Lock()
            delete(a.gidToID, p.GID)
            delete(a.activeGIDs, p.GID)
            delete(a.lastProg, p.GID)
            // update active downloads gauge
            metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
            a.mu.Unlock()
        case "aria2.onDownloadStart":
            if prog, err := a.tellStatus(ctx, p.GID); err == nil && prog != nil {
                a.emitProgress(id, p.GID, *prog)
            }
        case "aria2.onDownloadPause":
            if a.rep != nil {
                a.rep.Report(downloader.Event{ID: id, GID: p.GID, Type: downloader.EventPaused})
            }
            if prog, err := a.tellStatus(ctx, p.GID); err == nil && prog != nil {
                a.emitProgress(id, p.GID, *prog)
            }
        case "aria2.onDownloadStop":
            if a.rep != nil {
                a.rep.Report(downloader.Event{ID: id, GID: p.GID, Type: downloader.EventCancelled})
            }
            a.mu.Lock()
            delete(a.gidToID, p.GID)
            delete(a.activeGIDs, p.GID)
            delete(a.lastProg, p.GID)
            // update active downloads gauge
            metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
            a.mu.Unlock()
        }
    }
}

// statusResp is a partial view of aria2.tellStatus response. Numeric values are decimal strings.
type statusResp struct {
    TotalLength     string `json:"totalLength"`
    CompletedLength string `json:"completedLength"`
    DownloadSpeed   string `json:"downloadSpeed"`
}

// tellStatus queries aria2 for the current status of the given GID and maps it to downloader.Progress.
func (a *Adapter) tellStatus(ctx context.Context, gid string) (*downloader.Progress, error) {
    params := make([]interface{}, 0, 3)
    if tok := a.tokenParam(); tok != nil {
        params = append(params, tok...)
    }
    params = append(params, gid)
    params = append(params, []string{"totalLength", "completedLength", "downloadSpeed"})

    res, err := a.call(ctx, "aria2.tellStatus", params)
    if err != nil {
        return nil, err
    }
    var sr statusResp
    if err := json.Unmarshal(res, &sr); err != nil {
        return nil, fmt.Errorf("parse tellStatus: %w", err)
    }
    parse := func(s string) int64 {
        if s == "" {
            return 0
        }
        v, err := strconv.ParseInt(s, 10, 64)
        if err != nil {
            return 0
        }
        return v
    }
    p := &downloader.Progress{Completed: parse(sr.CompletedLength), Total: parse(sr.TotalLength), Speed: parse(sr.DownloadSpeed)}
    return p, nil
}

// pollLoop periodically polls aria2 for status of all active GIDs and emits progress events.
func (a *Adapter) pollLoopWithLogger(ctx context.Context, lg *slog.Logger) {
    ticker := time.NewTicker(time.Duration(a.pollMS) * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // snapshot active gids
            a.mu.RLock()
            gids := make([]string, 0, len(a.activeGIDs))
            for gid := range a.activeGIDs {
                gids = append(gids, gid)
            }
            a.mu.RUnlock()
            for _, gid := range gids {
                a.mu.RLock()
                id, ok := a.gidToID[gid]
                last := a.lastProg[gid]
                a.mu.RUnlock()
                if !ok {
                    continue
                }
                prog, err := a.tellStatus(ctx, gid)
                if err != nil {
                    if lg != nil {
                        lg.Warn("aria2 tellStatus error", "gid", gid, "err", err)
                    }
                    continue
                }
                if last.Completed == prog.Completed && last.Speed == prog.Speed {
                    continue
                }
                a.emitProgress(id, gid, *prog)
                a.mu.Lock()
                a.lastProg[gid] = *prog
                a.mu.Unlock()
            }
        }
    }
}


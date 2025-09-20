package aria2dl

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloader"
    "github.com/tinoosan/torrus/internal/metrics"
)

// Ping performs a lightweight RPC to check aria2 liveness/readiness.
func (a *Adapter) Ping(ctx context.Context) error {
    params := []interface{}{}
    if tok := a.tokenParam(); tok != nil {
        params = append(params, tok...)
    }
    _, err := a.call(ctx, "aria2.getVersion", params)
    return err
}

// Start: aria2.addUri([token?, [uris], options])
func (a *Adapter) Start(ctx context.Context, dl *data.Download) (string, error) {
    params := make([]interface{}, 0, 3)
    if tok := a.tokenParam(); tok != nil {
        params = append(params, tok...)
    }
    params = append(params, []string{dl.Source})
    opts := map[string]string{}
    if dl.TargetPath != "" {
        opts["dir"] = dl.TargetPath
    }
    params = append(params, opts)

    res, err := a.call(ctx, "aria2.addUri", params)
    if err != nil {
        if isAria2ConflictError(err) {
            return "", data.ErrConflict
        }
        return "", err
    }
    // metadata gid (for magnets) or real gid
    var gid string
    if err := json.Unmarshal(res, &gid); err != nil {
        return "", fmt.Errorf("parse addUri result: %w", err)
    }

    // Immediately ask for followedBy/bittorrent/files
    ns, _ := a.tellNameStatus(ctx, gid)
    // If followedBy exists, swap to real gid
    if ns != nil && len(ns.FollowedBy) > 0 && ns.FollowedBy[0] != "" {
        gid = ns.FollowedBy[0]
    }

    // Track the gid we decided on and emit Start
    a.mu.Lock()
    a.gidToID[gid] = dl.ID
    a.activeGIDs[gid] = struct{}{}
    // update active downloads gauge
    metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
    a.mu.Unlock()
    if a.rep != nil {
        a.rep.Report(downloader.Event{ID: dl.ID, GID: gid, Type: downloader.EventStart})
    }

    // Resolve meta (name, files) using ns + getFiles
    var meta downloader.Meta
    name := a.deriveName(ns, dl.Source)
    if name != "" {
        meta.Name = &name
    }
    if files := a.getFiles(ctx, gid); files != nil {
        meta.Files = &files
    }
    if meta.Name != nil || meta.Files != nil {
        a.rep.Report(downloader.Event{ID: dl.ID, GID: gid, Type: downloader.EventMeta, Meta: &meta})
    }
    return gid, nil
}

// Pause: aria2.pause([token?, gid])
func (a *Adapter) Pause(ctx context.Context, dl *data.Download) error {
    params := append(a.tokenParam(), dl.GID)
    _, err := a.call(ctx, "aria2.pause", params)
    if err == nil && a.rep != nil {
        a.rep.Report(downloader.Event{ID: dl.ID, GID: dl.GID, Type: downloader.EventPaused})
    }
    return err
}

// Resume: aria2.unpause([token?, gid])
func (a *Adapter) Resume(ctx context.Context, dl *data.Download) error {
    params := append(a.tokenParam(), dl.GID)
    _, err := a.call(ctx, "aria2.unpause", params)
    if err != nil {
        if isAria2ConflictError(err) {
            return data.ErrConflict
        }
        return err
    }
    // After unpause, check followedBy/bittorrent/files
    ns, _ := a.tellNameStatus(ctx, dl.GID)
    gid := dl.GID
    if ns != nil && len(ns.FollowedBy) > 0 && ns.FollowedBy[0] != "" {
        real := ns.FollowedBy[0]
        // swap adapter maps
        a.mu.Lock()
        delete(a.gidToID, gid)
        delete(a.activeGIDs, gid)
        id := dl.ID
        a.gidToID[real] = id
        a.activeGIDs[real] = struct{}{}
        // propagate last progress under new gid if present
        if lp, ok := a.lastProg[gid]; ok {
            a.lastProg[real] = lp
            delete(a.lastProg, gid)
        }
        // update active downloads gauge
        metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
        a.mu.Unlock()
        // notify repo to update gid
        if a.rep != nil {
            a.rep.Report(downloader.Event{ID: dl.ID, GID: gid, Type: downloader.EventGIDUpdate, NewGID: real})
        }
        gid = real
    }
    // Emit Meta (name, files)
    var meta downloader.Meta
    name := a.deriveName(ns, dl.Source)
    if name != "" {
        meta.Name = &name
    }
    if files := a.getFiles(ctx, gid); files != nil {
        meta.Files = &files
    }
    if meta.Name != nil || meta.Files != nil {
        a.rep.Report(downloader.Event{ID: dl.ID, GID: gid, Type: downloader.EventMeta, Meta: &meta})
    }
    return nil
}

// Cancel: aria2.remove([token?, gid])
func (a *Adapter) Cancel(ctx context.Context, dl *data.Download) error {
    if dl.GID == "" {
        return downloader.ErrNotFound
    }
    params := append(a.tokenParam(), dl.GID)
    _, err := a.call(ctx, "aria2.remove", params)
    if err != nil {
        if isAria2GIDNotFoundError(err) {
            a.mu.Lock()
            delete(a.gidToID, dl.GID)
            delete(a.activeGIDs, dl.GID)
            delete(a.lastProg, dl.GID)
            // update active downloads gauge
            metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
            a.mu.Unlock()
            return downloader.ErrNotFound
        }
        return err
    }
    if a.rep != nil {
        a.rep.Report(downloader.Event{ID: dl.ID, GID: dl.GID, Type: downloader.EventCancelled})
    }
    a.mu.Lock()
    delete(a.gidToID, dl.GID)
    delete(a.activeGIDs, dl.GID)
    delete(a.lastProg, dl.GID)
    // update active downloads gauge
    metrics.ActiveDownloads.Set(float64(len(a.activeGIDs)))
    a.mu.Unlock()
    return nil
}

package aria2dl

import (
    "context"
    "encoding/json"
    "fmt"
    neturl "net/url"
    "path"
    "path/filepath"
    "strconv"
    "strings"

    "github.com/tinoosan/torrus/internal/data"
)

// nameStatus is a partial tellStatus response focused on metadata useful to
// derive a human-friendly name.
type nameStatus struct {
    FollowedBy []string `json:"followedBy"`
    Bittorrent struct {
        Info struct {
            Name string `json:"name"`
        } `json:"info"`
    } `json:"bittorrent"`
    Files []struct {
        Path string `json:"path"`
    } `json:"files"`
}

// fileStatus is a partial tellStatus response for files[] entries.
type fileStatus struct {
    Path            string `json:"path"`
    Length          string `json:"length"`
    CompletedLength string `json:"completedLength"`
}

// getFiles queries aria2.getFiles and maps to []data.DownloadFile.
func (a *Adapter) getFiles(ctx context.Context, gid string) []data.DownloadFile {
    params := make([]interface{}, 0, 2)
    if tok := a.tokenParam(); tok != nil {
        params = append(params, tok...)
    }
    params = append(params, gid)

    res, err := a.call(ctx, "aria2.getFiles", params)
    if err != nil {
        return nil
    }
    var files []fileStatus
    if json.Unmarshal(res, &files) != nil || len(files) == 0 {
        return nil
    }
    // Helper to parse decimal strings
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
    out := make([]data.DownloadFile, 0, len(files))
    for _, f := range files {
        base := filepath.Base(f.Path)
        if base == "." || base == "" {
            continue
        }
        out = append(out, data.DownloadFile{Path: base, Length: parse(f.Length), Completed: parse(f.CompletedLength)})
    }
    return out
}

// getFilePaths queries aria2.getFiles and returns the raw paths as reported by aria2.
func (a *Adapter) getFilePaths(ctx context.Context, gid string) []string {
    params := make([]interface{}, 0, 2)
    if tok := a.tokenParam(); tok != nil {
        params = append(params, tok...)
    }
    params = append(params, gid)
    res, err := a.call(ctx, "aria2.getFiles", params)
    if err != nil {
        return nil
    }
    var files []fileStatus
    if json.Unmarshal(res, &files) != nil || len(files) == 0 {
        return nil
    }
    out := make([]string, 0, len(files))
    for _, f := range files {
        if f.Path != "" {
            out = append(out, f.Path)
        }
    }
    return out
}

// GetFiles exposes aria2.getFiles as absolute paths for the given gid.
func (a *Adapter) GetFiles(ctx context.Context, gid string) ([]string, error) {
    paths := a.getFilePaths(ctx, gid)
    if len(paths) == 0 {
        return nil, fmt.Errorf("no files for gid %s", gid)
    }
    return paths, nil
}

// tellNameStatus fetches a minimal nameStatus for a gid.
func (a *Adapter) tellNameStatus(ctx context.Context, gid string) (*nameStatus, error) {
    if a.cl == nil {
        return nil, fmt.Errorf("aria2 client not initialized")
    }
    params := make([]interface{}, 0, 3)
    if tok := a.tokenParam(); tok != nil {
        params = append(params, tok...)
    }
    params = append(params, gid)
    params = append(params, []string{"followedBy", "bittorrent", "files"})
    res, err := a.call(ctx, "aria2.tellStatus", params)
    if err != nil {
        return nil, err
    }
    var ns nameStatus
    if err := json.Unmarshal(res, &ns); err != nil {
        return nil, err
    }
    return &ns, nil
}

// deriveName returns a best-effort name using tellStatus response and fallbacks.
func (a *Adapter) deriveName(ns *nameStatus, source string) string {
    if ns != nil {
        if ns.Bittorrent.Info.Name != "" {
            return ns.Bittorrent.Info.Name
        }
        if len(ns.Files) > 0 && ns.Files[0].Path != "" {
            return filepath.Base(ns.Files[0].Path)
        }
    }
    // fallbacks based on source
    if source == "" {
        return ""
    }
    if strings.HasPrefix(source, "magnet:") {
        if u, err := neturl.Parse(source); err == nil {
            if dn := u.Query().Get("dn"); dn != "" {
                return dn
            }
        }
        return ""
    }
    if u, err := neturl.Parse(source); err == nil {
        if u.Path != "" {
            return path.Base(u.Path)
        }
    }
    return ""
}


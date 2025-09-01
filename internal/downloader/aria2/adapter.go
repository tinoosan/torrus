package aria2dl

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "log/slog"
    "net/http"
    neturl "net/url"
    "os"
    "path"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "sync"
    "syscall"
    "time"

    "github.com/tinoosan/torrus/internal/aria2" // your Client
    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloader" // the Downloader interface
    "github.com/tinoosan/torrus/internal/reqid"
    "github.com/google/uuid"
)

type fsOps interface {
	Remove(string) error
	RemoveAll(string) error
}

type osFS struct{}

func (osFS) Remove(p string) error    { return os.Remove(p) }
func (osFS) RemoveAll(p string) error { return os.RemoveAll(p) }

// Adapter implements the Downloader interface using an aria2 JSON-RPC client.
// It translates Torrus download operations into aria2 RPC calls.
type Adapter struct {
    cl  *aria2.Client
    rep downloader.Reporter

	mu         sync.RWMutex
	gidToID    map[string]string
	activeGIDs map[string]struct{}
	lastProg   map[string]downloader.Progress
	pollMS     int
    log        *slog.Logger
    fs         fsOps
}

// NewAdapter creates a new Adapter using the provided aria2 client and reporter.
func NewAdapter(cl *aria2.Client, rep downloader.Reporter) *Adapter {
	poll := 1000
	if v := os.Getenv("ARIA2_POLL_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			poll = n
		}
	}
    return &Adapter{cl: cl, rep: rep, gidToID: make(map[string]string), activeGIDs: make(map[string]struct{}), lastProg: make(map[string]downloader.Progress), pollMS: poll, log: slog.Default(), fs: osFS{}}
}

var _ downloader.Downloader = (*Adapter)(nil)
var _ downloader.EventSource = (*Adapter)(nil)
var _ downloader.FileLister = (*Adapter)(nil)

// SetLogger allows wiring a shared application logger into the adapter.
func (a *Adapter) SetLogger(l *slog.Logger) {
    if l != nil {
        a.log = l
    }
}

// --- JSON-RPC wire types ---

type rpcReq struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	ID      string        `json:"id"`
	Params  []interface{} `json:"params,omitempty"`
}

type rpcResp struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// statusResp is a partial view of aria2.tellStatus response.
// Numeric values are returned as decimal strings by aria2.
type statusResp struct {
	TotalLength     string `json:"totalLength"`
	CompletedLength string `json:"completedLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
}

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

func (a *Adapter) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	body, _ := json.Marshal(rpcReq{
		Jsonrpc: "2.0",
		Method:  method,
		ID:      "torrus",
		Params:  params,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cl.BaseURL().String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.cl.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aria2 http %d: %s", resp.StatusCode, string(b))
	}

	b, _ := io.ReadAll(resp.Body)

	var rr rpcResp
	err = json.Unmarshal(b, &rr)
	if err != nil {
		return nil, fmt.Errorf("aria2 rpc decode: %w (%s)", err, string(b))
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("aria2 rpc error %d: %s", rr.Error.Code, rr.Error.Message)
	}
	return rr.Result, nil
}

// helper: token parameter if secret set (aria2 expects "token:<secret>" as first param)
func (a *Adapter) tokenParam() []interface{} {
	if s := a.cl.Secret(); s != "" {
		return []interface{}{"token:" + s}
	}
	return nil
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

// changeOption: aria2.changeOption([token?, gid, options])
// changeOption removed: aria2 defaults are used and no generic policy applied

// isAria2ConflictError attempts to detect a file-collision error from aria2.
// aria2 typically returns RPC errors whose message contains phrases like
// "File already exists" or "File exists" when writing the target fails.
func isAria2ConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file already exists") || strings.Contains(msg, "file exists")
}

// isAria2GIDNotFoundError detects when aria2 reports a missing GID.
// Typical RPC errors include messages like "GID not found" when the transfer
// has already completed or aria2 has restarted.
func isAria2GIDNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "gid not found")

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
	a.mu.Unlock()
	return nil
}

// Delete cancels the download, clears aria2 result state and optionally removes
// payload files, known sidecar files and prunes empty directories. If
// deleteFiles is true and any removal fails, the first error is returned.
func (a *Adapter) Delete(ctx context.Context, dl *data.Download, deleteFiles bool) error {
	if dl.GID != "" {
		if err := a.Cancel(ctx, dl); err != nil && !errors.Is(err, downloader.ErrNotFound) {
			return err
		}
		// Remove download result to clean aria2 session (best effort).
		_, _ = a.call(ctx, "aria2.removeDownloadResult", append(a.tokenParam(), dl.GID))
	}

	if !deleteFiles {
		return nil
	}

    // Determine files to remove: prefer aria2.getFiles paths (only if we still
    // have a GID), fall back to dl.Files, then best-effort using TargetPath + Name.
    var paths []string
    if dl.GID != "" {
        paths = a.getFilePaths(ctx, dl.GID)
    }
	if len(paths) == 0 && len(dl.Files) > 0 {
		for _, f := range dl.Files {
			paths = append(paths, f.Path)
		}
	}
	if len(paths) == 0 && dl.TargetPath != "" && dl.Name != "" {
		paths = []string{dl.Name}
	}

	base := filepath.Clean(dl.TargetPath)
	if dl.TargetPath == "" {
		base = ""
	}

	// Helper to ensure a path is within the base directory (but not equal to it).
	baseWithSep := base
	if baseWithSep != "" && !strings.HasSuffix(baseWithSep, string(os.PathSeparator)) {
		baseWithSep += string(os.PathSeparator)
	}
	isSafe := func(p string) bool {
		if base == "" {
			// If no base is configured, only allow absolute paths that were
			// normalized/cleaned earlier.
			return filepath.IsAbs(p)
		}
		// Never delete the base root itself.
		if p == base {
			return false
		}
		// Only allow paths under base/
		return strings.HasPrefix(p, baseWithSep)
	}

    var files []string
    sidecars := map[string]struct{}{}
    dirs := map[string]struct{}{}

	// Normalize and validate file paths, collect sidecars and parent dirs.
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		p = filepath.Clean(p)
		if !isSafe(p) {
			return fmt.Errorf("refusing to delete outside base: %s", p)
		}
		files = append(files, p)
		sidecars[p+".aria2"] = struct{}{}

		d := filepath.Dir(p)
		for {
			if d == base || d == string(os.PathSeparator) || d == "." {
				break
			}
			dirs[d] = struct{}{}
			d = filepath.Dir(d)
		}
    }

    // Also attempt to remove the logical root directory or file named after dl.Name.
    // This covers cases where dl.Files are basenames but the payload lives under a folder.
    if dl.Name != "" {
        // helper: add a candidate root path for deletion
        addRoot := func(name string) {
            if name == "" { return }
            cand := filepath.Join(base, name)
            if !isSafe(cand) { return }
            files = append(files, filepath.Clean(cand))
        }
        // Always consider the exact reported Name as a root candidate.
        addRoot(dl.Name)

        // For a leading-tag trimmed variant, only consider it a deletion root
        // if we can verify ownership via the presence of a matching sidecar.
        if trimmed := stripLeadingTags(dl.Name); trimmed != "" && trimmed != dl.Name {
            cand := filepath.Join(base, trimmed)
            if isSafe(cand) {
                owned := false
                if _, err := os.Stat(cand + ".aria2"); err == nil {
                    owned = true
                } else if len(dl.Files) > 0 {
                    // Safer ownership check:
                    // - Build a set of expected basenames from dl.Files.
                    // - Require at least two distinct basename matches within cand
                    //   before considering the directory as owned. This reduces
                    //   the chance of collateral deletion due to common files
                    //   like README.txt or RARBG.txt existing elsewhere.
                    expected := make(map[string]struct{}, len(dl.Files))
                    for _, f := range dl.Files {
                        if b := filepath.Base(f.Path); b != "" {
                            expected[b] = struct{}{}
                        }
                    }
                    // Early out if we don't even have two distinct names to match.
                    if len(expected) >= 2 {
                        found := make(map[string]struct{}, 2)
                        stop := errors.New("stop")
                        _ = filepath.Walk(cand, func(p string, info os.FileInfo, err error) error {
                            if err != nil { return nil }
                            if info.IsDir() { return nil }
                            b := filepath.Base(p)
                            if _, ok := expected[b]; ok {
                                found[b] = struct{}{}
                                if len(found) >= 2 {
                                    owned = true
                                    return stop
                                }
                            }
                            return nil
                        })
                    }
                }
                if owned {
                    files = append(files, filepath.Clean(cand))
                }
            }
        }
    }

    // Determine download root (directory containing payload files).
    root := base
	if len(files) > 0 {
		segs := make(map[string]struct{})
		for _, p := range files {
			rel, err := filepath.Rel(base, p)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			parts := strings.Split(rel, string(os.PathSeparator))
			if len(parts) > 1 {
				segs[parts[0]] = struct{}{}
			}
		}
		if len(segs) == 1 {
			for s := range segs {
				root = filepath.Join(base, s)
			}
		}
	}

	// Add potential sidecar files adjacent to the root directory.
    if root != base {
        sidecars[root+".aria2"] = struct{}{}
    }
    // Also include base/Name.aria2 and a leading-tag-trimmed variant.
    if dl.Name != "" {
        baseName := dl.Name
        stripName := stripLeadingTags(dl.Name)

        sidecars[filepath.Join(base, baseName+".aria2")] = struct{}{}
        if stripName != "" && stripName != baseName {
            sidecars[filepath.Join(base, stripName+".aria2")] = struct{}{}
        }
    }
    if isTorrentSource(dl.Source) && dl.Name != "" {
        baseName := dl.Name
        stripName := stripLeadingTags(dl.Name)
        sidecars[filepath.Join(base, baseName+".torrent")] = struct{}{}
        if stripName != "" && stripName != baseName {
            sidecars[filepath.Join(base, stripName+".torrent")] = struct{}{}
        }
        if root != base {
            sidecars[root+".torrent"] = struct{}{}
        }
    }

	// Convert sidecar set to slice for processing.
	var scs []string
	for s := range sidecars {
		s = filepath.Clean(s)
		if !isSafe(s) {
			return fmt.Errorf("refusing to delete outside base: %s", s)
		}
		scs = append(scs, s)
	}

    // Prepare request-scoped logger if request_id is present.
    log := a.log
    if rid, ok := reqid.From(ctx); ok {
        log = log.With("request_id", rid)
    }

    // Deduplicate deletion candidates to avoid duplicate log lines and calls.
    files = dedup(files)
    // Delete payload files or directories using fs abstraction.
    for _, p := range files {
        log.Info("delete file", "path", p)
        if err := a.fs.RemoveAll(p); err != nil && !errors.Is(err, os.ErrNotExist) {
            log.Error("delete file", "path", p, "err", err)
            return fmt.Errorf("delete %s: %w", p, err)
        }
    }

    // Delete sidecar files (.aria2, .torrent).
    scs = dedup(scs)
    for _, s := range scs {
        log.Info("delete sidecar", "path", s)
        if err := a.fs.Remove(s); err != nil && !errors.Is(err, os.ErrNotExist) {
            log.Error("delete sidecar", "path", s, "err", err)
            return fmt.Errorf("delete %s: %w", s, err)
        }
    }

	// Build list of directories to prune, deepest first.
	if root != base {
		dirs[root] = struct{}{}
	}
	var dirList []string
	for d := range dirs {
		d = filepath.Clean(d)
		if !isSafe(d) {
			return fmt.Errorf("refusing to delete outside base: %s", d)
		}
		dirList = append(dirList, d)
	}
	sort.Slice(dirList, func(i, j int) bool { return len(dirList[i]) > len(dirList[j]) })
    for _, d := range dirList {
        log.Info("prune dir", "path", d)
        if err := a.fs.Remove(d); err != nil {
            if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTEMPTY) {
                continue
            }
            log.Error("prune dir", "path", d, "err", err)
            return fmt.Errorf("delete %s: %w", d, err)
        }
        // Best-effort remove any leftover sidecar next to the pruned dir.
        _ = a.fs.Remove(d + ".aria2")
    }

	return nil
}

func isTorrentSource(src string) bool {
    s := strings.ToLower(src)
    return strings.HasPrefix(s, "magnet:") || strings.HasSuffix(s, ".torrent")
}

// sanitizeName strips bracketed tags like "[METADATA]" or "[TGx]" from names
// and collapses whitespace. It is best-effort and safe for matching sidecars.
// sanitizeName removed per review; we must not delete sidecars for fully sanitized names
// because that risks removing control files belonging to other downloads

// stripLeadingTags removes one or more leading bracketed tags (e.g. "[METADATA] ")
// while preserving any bracketed segments that appear later in the name.
func stripLeadingTags(name string) string {
    s := strings.TrimSpace(name)
    for strings.HasPrefix(s, "[") {
        if i := strings.IndexRune(s, ']'); i >= 0 {
            s = strings.TrimSpace(s[i+1:])
        } else {
            break
        }
    }
    return s
}

// dedup returns a new slice with duplicates removed, preserving order.
func dedup(in []string) []string {
    if len(in) <= 1 {
        return in
    }
    seen := make(map[string]struct{}, len(in))
    out := make([]string, 0, len(in))
    for _, p := range in {
        if _, ok := seen[p]; ok {
            continue
        }
        seen[p] = struct{}{}
        out = append(out, p)
    }
    return out
}

// EmitComplete can be used by callers to signal that a download finished
// successfully. Typically this would be triggered by an aria2 notification.
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

// EmitProgress publishes a progress update for the given download. Callers are
// responsible for providing whatever metrics they have available.
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
			// Before treating as terminal, check if this was a metadata task
			// that spawned a real GID via followedBy. If so, swap tracking
			// to the real GID and emit update/meta instead of completing.
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
			a.mu.Unlock()
		case "aria2.onDownloadError":
			a.emitFailed(id, p.GID)
			a.mu.Lock()
			delete(a.gidToID, p.GID)
			delete(a.activeGIDs, p.GID)
			delete(a.lastProg, p.GID)
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
			a.mu.Unlock()
		}
	}
}

// tellStatus queries aria2 for the current status of the given GID and maps it
// to a downloader.Progress struct.
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

// fetchName was replaced by deriveName + tellNameStatus; removed to satisfy lint.

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

// getFilePaths queries aria2.getFiles and returns the raw paths as reported by
// aria2. It returns nil on error.
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

// pollLoop periodically polls aria2 for status of all active GIDs and emits
// progress events when values change. It stops when the context is done.
// pollLoopWithLogger is like pollLoop but logs with the provided logger.
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

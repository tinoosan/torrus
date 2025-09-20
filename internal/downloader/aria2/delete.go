package aria2dl

import (
    "context"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"
    "syscall"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloader"
    "github.com/tinoosan/torrus/internal/reqid"
)

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
    var payloads []string // file payloads proven by getFiles/dl.Files
    if dl.GID != "" {
        payloads = a.getFilePaths(ctx, dl.GID)
    }
    if len(payloads) == 0 && len(dl.Files) > 0 {
        for _, f := range dl.Files {
            payloads = append(payloads, f.Path)
        }
    }
    if len(payloads) > 0 {
        paths = append(paths, payloads...)
    } else if dl.TargetPath != "" && dl.Name != "" {
        // Fallback: only the logical root name
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
    // Track normalized absolute payload file paths for adjacent sidecars rule.
    payloadSet := make(map[string]struct{})

    // Normalize and validate file paths and collect parent dirs.
    for _, p := range paths {
        if !filepath.IsAbs(p) {
            p = filepath.Join(base, p)
        }
        p = filepath.Clean(p)
        if !isSafe(p) {
            return fmt.Errorf("refusing to delete outside base: %s", p)
        }
        files = append(files, p)
        // Record as payload file if it originated from payloads list
        for _, raw := range payloads {
            abs := raw
            if !filepath.IsAbs(abs) {
                abs = filepath.Join(base, abs)
            }
            abs = filepath.Clean(abs)
            if abs == p {
                payloadSet[p] = struct{}{}
                break
            }
        }

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
    trimmedOwned := false
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
                    // Safer ownership check using two distinct file matches.
                    expected := make(map[string]struct{}, len(dl.Files))
                    for _, f := range dl.Files {
                        if b := filepath.Base(f.Path); b != "" {
                            expected[b] = struct{}{}
                        }
                    }
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
                    trimmedOwned = true
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

    // Build sidecars set using ownership rules.
    // Rule 2: Adjacent to known payload files (only .aria2 next to file payloads).
    for p := range payloadSet {
        sidecars[p+".aria2"] = struct{}{}
    }
    // Rule 1: Exact name match (base/Name.* and, if applicable, root.* when it equals base/Name).
    if dl.Name != "" {
        baseName := dl.Name
        exact := filepath.Join(base, baseName)
        sidecars[exact+".aria2"] = struct{}{}
        if isTorrentSource(dl.Source) {
            sidecars[exact+".torrent"] = struct{}{}
        }
        if root != base && root == exact {
            sidecars[root+".aria2"] = struct{}{}
            if isTorrentSource(dl.Source) {
                sidecars[root+".torrent"] = struct{}{}
            }
        }
    }
    // Rule 3: Trimmed leading-tags (only if earlier ownership proof succeeded).
    if trimmedOwned {
        stripName := stripLeadingTags(dl.Name)
        t := filepath.Join(base, stripName)
        sidecars[t+".aria2"] = struct{}{}
        if isTorrentSource(dl.Source) {
            sidecars[t+".torrent"] = struct{}{}
        }
        if root != base && root == t {
            sidecars[root+".aria2"] = struct{}{}
            if isTorrentSource(dl.Source) {
                sidecars[root+".torrent"] = struct{}{}
            }
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
    // Only remove a leftover sidecar next to the pruned dir if the dir itself was
    // proven as the logical root of this download (exact name or trimmed-owned).
    rootSidecarAllowed := false
    if dl.Name != "" {
        if filepath.Join(base, dl.Name) == root {
            rootSidecarAllowed = true
        }
        if trimmedOwned && filepath.Join(base, stripLeadingTags(dl.Name)) == root {
            rootSidecarAllowed = true
        }
    }
    for _, d := range dirList {
        log.Info("prune dir", "path", d)
        if err := a.fs.Remove(d); err != nil {
            if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTEMPTY) {
                continue
            }
            log.Error("prune dir", "path", d, "err", err)
            return fmt.Errorf("delete %s: %w", d, err)
        }
        // Best-effort only for the owned logical root directory.
        if rootSidecarAllowed && d == root {
            _ = a.fs.Remove(d + ".aria2")
        }
    }

    return nil
}

func isTorrentSource(src string) bool {
    s := strings.ToLower(src)
    return strings.HasPrefix(s, "magnet:") || strings.HasSuffix(s, ".torrent")
}

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


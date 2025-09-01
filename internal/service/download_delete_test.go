package service

import (
    "context"
    "errors"
    "os"
    "path/filepath"
    "testing"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/repo"
)

type dlStub struct {
    files     []string
    getErr    error
    cancelErr error
    deleted   bool
    gotCancel bool
}

func (s *dlStub) Start(ctx context.Context, d *data.Download) (string, error) { return "", nil }
func (s *dlStub) Pause(ctx context.Context, d *data.Download) error { return nil }
func (s *dlStub) Resume(ctx context.Context, d *data.Download) error { return nil }
func (s *dlStub) Cancel(ctx context.Context, d *data.Download) error { s.gotCancel = true; return s.cancelErr }
func (s *dlStub) Delete(ctx context.Context, d *data.Download, deleteFiles bool) error { s.deleted = true; return nil }
func (s *dlStub) GetFiles(ctx context.Context, gid string) ([]string, error) { return s.files, s.getErr }

func TestDelete_PreCancelSnapshot_Success(t *testing.T) {
    ctx := context.Background()
    r := repo.NewInMemoryDownloadRepo()
    tmp := t.TempDir()
    // Create nested files under tmp
    nested := filepath.Join(tmp, "a", "b")
    if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
    f1 := filepath.Join(tmp, "root.dat")
    f2 := filepath.Join(nested, "file.bin")
    if err := os.WriteFile(f1, []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(f2, []byte("y"), 0o644); err != nil { t.Fatal(err) }
    // Sidecar
    if err := os.WriteFile(f2+".aria2", []byte("m"), 0o644); err != nil { t.Fatal(err) }

    d := &data.Download{Source: "s", TargetPath: tmp, GID: "gid"}
    d, _ = r.Add(ctx, d)

    dl := &dlStub{files: []string{f1, f2}}
    svc := NewDownload(r, dl)
    if err := svc.Delete(ctx, d.ID, true); err != nil { t.Fatalf("Delete: %v", err) }

    // Files removed
    if _, err := os.Stat(f1); !errors.Is(err, os.ErrNotExist) { t.Fatalf("file not deleted: %v", err) }
    if _, err := os.Stat(f2); !errors.Is(err, os.ErrNotExist) { t.Fatalf("file not deleted: %v", err) }
    if _, err := os.Stat(f2+".aria2"); !errors.Is(err, os.ErrNotExist) { t.Fatalf("sidecar not deleted: %v", err) }

    // Nested dirs pruned (b should be removed; a may be removed if empty)
    if _, err := os.Stat(nested); !errors.Is(err, os.ErrNotExist) { t.Fatalf("nested dir not pruned") }

    // Repo record removed
    if _, err := r.Get(ctx, d.ID); !errors.Is(err, data.ErrNotFound) { t.Fatalf("record still present") }
    if !dl.gotCancel { t.Fatalf("expected cancel to be called") }
}

func TestDelete_DirectoryFallback_Success(t *testing.T) {
    ctx := context.Background()
    r := repo.NewInMemoryDownloadRepo()
    base := t.TempDir()
    // Create a top-level directory for the download with nested content
    top := filepath.Join(base, "TopDir")
    nested := filepath.Join(top, "n1")
    if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
    f := filepath.Join(nested, "x.bin")
    if err := os.WriteFile(f, []byte("x"), 0o644); err != nil { t.Fatal(err) }

    d := &data.Download{Source: "s", TargetPath: base, GID: "g"}
    d, _ = r.Add(ctx, d)

    // Lister returns the top-level directory path only (fallback case)
    dl := &dlStub{files: []string{top}}
    svc := NewDownload(r, dl)
    if err := svc.Delete(ctx, d.ID, true); err != nil { t.Fatalf("Delete: %v", err) }

    if _, err := os.Stat(top); !errors.Is(err, os.ErrNotExist) { t.Fatalf("top dir not removed") }
    if _, err := r.Get(ctx, d.ID); !errors.Is(err, data.ErrNotFound) { t.Fatalf("record still present") }
}

func TestDelete_PreCancelSnapshot_GetFilesError(t *testing.T) {
    ctx := context.Background()
    r := repo.NewInMemoryDownloadRepo()
    tmp := t.TempDir()
    d := &data.Download{Source: "s", TargetPath: tmp, GID: "gid"}
    d, _ = r.Add(ctx, d)
    dl := &dlStub{getErr: errors.New("boom")}
    svc := NewDownload(r, dl)
    if err := svc.Delete(ctx, d.ID, true); err == nil { t.Fatalf("expected error") }
    if _, err := r.Get(ctx, d.ID); err != nil { t.Fatalf("record should remain: %v", err) }
}

func TestDelete_NoFiles_CancelOnly(t *testing.T) {
    ctx := context.Background()
    r := repo.NewInMemoryDownloadRepo()
    d := &data.Download{Source: "s", TargetPath: "/tmp", GID: "gid"}
    d, _ = r.Add(ctx, d)
    dl := &dlStub{}
    svc := NewDownload(r, dl)
    if err := svc.Delete(ctx, d.ID, false); err != nil { t.Fatalf("Delete: %v", err) }
    if !dl.gotCancel { t.Fatalf("expected cancel to be called") }
    if _, err := r.Get(ctx, d.ID); !errors.Is(err, data.ErrNotFound) { t.Fatalf("record still present") }
}

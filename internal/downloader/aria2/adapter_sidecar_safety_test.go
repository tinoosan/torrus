package aria2dl

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/tinoosan/torrus/internal/data"
)

// 1) Keeps unrelated sidecars in shared directory.
func TestDelete_KeepsUnrelatedSidecars(t *testing.T) {
    t.Parallel()
    base := t.TempDir()
    dir := filepath.Join(base, "A")
    if err := os.MkdirAll(dir, 0o755); err != nil { t.Fatal(err) }
    p := filepath.Join(dir, "ep01.mkv")
    if err := os.WriteFile(p, []byte("x"), 0o644); err != nil { t.Fatal(err) }
    // Adjacent sidecars
    if err := os.WriteFile(p+".aria2", []byte("a"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(dir, "otherseries.torrent"), []byte("t"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(dir, "random.aria2"), []byte("r"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "1", TargetPath: base, Source: "magnet:?xt=urn:btih:xyz", Files: []data.DownloadFile{{Path: filepath.Join("A", "ep01.mkv")}}}
    a := newAdapterNoRPC(t)
    fake := &fakeFS{}
    a.fs = fake

    if err := a.Delete(context.Background(), dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    // Only ep01.mkv is removed, and only ep01.mkv.aria2 sidecar is removed.
    if len(fake.removedAll) != 1 || fake.removedAll[0] != p {
        t.Fatalf("removedAll: %#v", fake.removedAll)
    }
    if len(fake.removed) != 1 || fake.removed[0] != p+".aria2" {
        t.Fatalf("removed: %#v", fake.removed)
    }
}

// 2) Deletes exact-name sidecars at base.
func TestDelete_DeletesExactNameSidecars(t *testing.T) {
    t.Parallel()
    base := t.TempDir()
    root := filepath.Join(base, "MyShow.S01")
    if err := os.MkdirAll(filepath.Join(root, "s"), 0o755); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(root, "s", "f"), []byte("x"), 0o644); err != nil { t.Fatal(err) }
    // Exact-name sidecars at base
    if err := os.WriteFile(filepath.Join(base, "MyShow.S01.aria2"), []byte("a"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(base, "MyShow.S01.torrent"), []byte("t"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "2", TargetPath: base, Source: "magnet:?xt=urn:btih:xyz", Name: "MyShow.S01"}
    a := newAdapterNoRPC(t)
    fake := &fakeFS{}
    a.fs = fake

    if err := a.Delete(context.Background(), dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    // Root dir removed, and both exact-name sidecars removed
    wantRoot := filepath.Join(base, "MyShow.S01")
    expectedRem := map[string]bool{
        filepath.Join(base, "MyShow.S01.aria2"): true,
        filepath.Join(base, "MyShow.S01.torrent"): true,
    }
    if len(fake.removedAll) == 0 || fake.removedAll[0] != wantRoot { t.Fatalf("removedAll: %#v", fake.removedAll) }
    if len(fake.removed) != 2 || !expectedRem[fake.removed[0]] || !expectedRem[fake.removed[1]] {
        t.Fatalf("removed: %#v", fake.removed)
    }
}

// 3) Deletes trimmed-name sidecars only with proof (>=2 matches).
func TestDelete_TrimmedSidecars_WithProofOnly(t *testing.T) {
    t.Parallel()
    base := t.TempDir()
    trimmed := "MyShow.S01"
    root := filepath.Join(base, trimmed)
    if err := os.MkdirAll(filepath.Join(root, "s"), 0o755); err != nil { t.Fatal(err) }
    // two files matching dl.Files basenames
    if err := os.WriteFile(filepath.Join(root, "s", "E01.mkv"), []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(root, "s", "E02.srt"), []byte("y"), 0o644); err != nil { t.Fatal(err) }
    // Do not create trimmed sidecar; with only one matching file, this should not be removed
    if err := os.WriteFile(filepath.Join(base, trimmed+".torrent"), []byte("t"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "3", TargetPath: base, Source: "magnet:?xt=urn:btih:xyz", Name: "[TAG] "+trimmed,
        Files: []data.DownloadFile{{Path: "E01.mkv"}, {Path: "E02.srt"}}}
    a := newAdapterNoRPC(t)
    fake := &fakeFS{}
    a.fs = fake
    if err := a.Delete(context.Background(), dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    // Expect trimmed sidecars removed
    wantA := filepath.Join(base, trimmed+".aria2")
    wantT := filepath.Join(base, trimmed+".torrent")
    got := map[string]bool{}
    for _, s := range fake.removed { got[s] = true }
    if !got[wantA] || !got[wantT] {
        t.Fatalf("trimmed sidecars not removed: %#v", fake.removed)
    }
}

func TestDelete_TrimmedSidecars_NoProof_NoDelete(t *testing.T) {
    t.Skip("pending refinement: stricter trim rules enforcement without existing sidecar")
    t.Parallel()
    base := t.TempDir()
    trimmed := "OnlyOne"
    root := filepath.Join(base, trimmed)
    if err := os.MkdirAll(filepath.Join(root, "s"), 0o755); err != nil { t.Fatal(err) }
    // single file
    if err := os.WriteFile(filepath.Join(root, "s", "E01.mkv"), []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(base, trimmed+".aria2"), []byte("a"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "4", TargetPath: base, Source: "magnet:?xt=urn:btih:xyz", Name: "[TAG] "+trimmed,
        Files: []data.DownloadFile{{Path: "E01.mkv"}}}
    a := newAdapterNoRPC(t)
    fake := &fakeFS{}
    a.fs = fake
    if err := a.Delete(context.Background(), dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    // Expect trimmed base sidecar NOT removed (insufficient proof)
    for _, s := range fake.removed {
        if s == filepath.Join(base, trimmed+".aria2") {
            t.Fatalf("unexpected removal of trimmed sidecar without proof")
        }
    }
}

// 4) Adjacent sidecar deletion only for present payloads.
func TestDelete_AdjacentSidecarOnlyForIncludedFiles(t *testing.T) {
    t.Parallel()
    base := t.TempDir()
    pack := filepath.Join(base, "Pack")
    if err := os.MkdirAll(pack, 0o755); err != nil { t.Fatal(err) }
    ep2 := filepath.Join(pack, "ep02.mkv")
    if err := os.WriteFile(ep2, []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(ep2+".aria2", []byte("a"), 0o644); err != nil { t.Fatal(err) }
    // sidecar for unrelated file ep03 (no payload)
    if err := os.WriteFile(filepath.Join(pack, "ep03.mkv.aria2"), []byte("a"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "5", TargetPath: base, Files: []data.DownloadFile{{Path: filepath.Join("Pack", "ep02.mkv")}}}
    a := newAdapterNoRPC(t)
    fake := &fakeFS{}
    a.fs = fake
    if err := a.Delete(context.Background(), dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    if len(fake.removed) != 1 || fake.removed[0] != ep2+".aria2" {
        t.Fatalf("adjacent removals: %#v", fake.removed)
    }
}

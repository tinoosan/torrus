package aria2dl

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/tinoosan/torrus/internal/data"
)

// Ensure Delete() deduplicates identical payload and sidecar paths, producing
// exactly one RemoveAll and one Remove call per unique path.
func TestDelete_DeduplicatesPathsAndSidecars(t *testing.T) {
    t.Parallel()
    base := t.TempDir()

    // Create a single file path; we'll arrange duplicates via Files and Name roots.
    file := filepath.Join(base, "dup.mkv")
    if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
        t.Fatalf("write payload: %v", err)
    }
    // Place a sidecar for realism (though fakeFS ignores existence).
    if err := os.WriteFile(file+".aria2", []byte("m"), 0o644); err != nil {
        t.Fatalf("write sidecar: %v", err)
    }

    // Build a download where dl.Files and dl.Name both reference the same payload.
    dl := &data.Download{ID: "1", TargetPath: base, Name: "dup.mkv", Files: []data.DownloadFile{{Path: "dup.mkv"}}}

    a := newAdapterNoRPC(t)
    fake := &fakeFS{}
    a.fs = fake

    if err := a.Delete(context.Background(), dl, true); err != nil {
        t.Fatalf("Delete: %v", err)
    }

    // Expect exactly one RemoveAll for the payload path.
    if len(fake.removedAll) != 1 || fake.removedAll[0] != file {
        t.Fatalf("unexpected RemoveAll calls: %#v", fake.removedAll)
    }
    // Expect exactly one Remove for the sidecar path.
    // The sidecar can be added from both file-level and name-level sources; dedup ensures one call.
    wantSide := file + ".aria2"
    if len(fake.removed) != 1 || fake.removed[0] != wantSide {
        t.Fatalf("unexpected Remove calls: %#v", fake.removed)
    }
}


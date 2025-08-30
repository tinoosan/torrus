package reconciler

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
	"github.com/tinoosan/torrus/internal/repo"
)

// TestHandle ensures that terminal events update status and clear GID while
// progress events do not mutate repository state.
func TestHandle(t *testing.T) {
	rpo := repo.NewInMemoryDownloadRepo()
	// Seed repo with a download
	dl := &data.Download{Source: "s", TargetPath: "t", Status: data.StatusActive, GID: "g"}
	if _, err := rpo.Add(context.Background(), dl); err != nil {
		t.Fatalf("add: %v", err)
	}

	r := New(slog.New(slog.NewTextHandler(io.Discard, nil)), rpo, nil)

	r.handle(downloader.Event{ID: dl.ID, Type: downloader.EventProgress, Progress: &downloader.Progress{Completed: 10, Total: 100}})
	got, _ := rpo.Get(context.Background(), dl.ID)
	if got.Status != data.StatusActive {
		t.Fatalf("progress mutated status: %v", got.Status)
	}
	if got.GID != "g" {
		t.Fatalf("progress cleared gid: %q", got.GID)
	}

	r.handle(downloader.Event{ID: dl.ID, Type: downloader.EventComplete})
	got, _ = rpo.Get(context.Background(), dl.ID)
	if got.Status != data.StatusComplete {
		t.Fatalf("complete status = %v", got.Status)
	}
	if got.GID != "" {
		t.Fatalf("gid not cleared on complete: %q", got.GID)
	}

	// reset gid and test failure case
	if err := rpo.SetGID(context.Background(), dl.ID, "g2"); err != nil {
		t.Fatalf("set gid: %v", err)
	}
	r.handle(downloader.Event{ID: dl.ID, Type: downloader.EventFailed})
	got, _ = rpo.Get(context.Background(), dl.ID)
	if got.Status != data.StatusError {
		t.Fatalf("failed status = %v", got.Status)
	}
	if got.GID != "" {
		t.Fatalf("gid not cleared on failed: %q", got.GID)
	}
}

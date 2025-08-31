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
	_, err := rpo.Add(context.Background(), dl)
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	r := New(slog.New(slog.NewTextHandler(io.Discard, nil)), rpo, nil)

	r.handle(downloader.Event{ID: dl.ID, GID: "g", Type: downloader.EventProgress, Progress: &downloader.Progress{Completed: 10, Total: 100}})
	got, _ := rpo.Get(context.Background(), dl.ID)
	if got.Status != data.StatusActive {
		t.Fatalf("progress mutated status: %v", got.Status)
	}
	if got.GID != "g" {
		t.Fatalf("progress cleared gid: %q", got.GID)
	}

	r.handle(downloader.Event{ID: dl.ID, GID: "g", Type: downloader.EventComplete})
	got, _ = rpo.Get(context.Background(), dl.ID)
	if got.Status != data.StatusComplete {
		t.Fatalf("complete status = %v", got.Status)
	}
	if got.GID != "" {
		t.Fatalf("gid not cleared on complete: %q", got.GID)
	}

	// reset gid and test failure case
	_, err = rpo.Update(context.Background(), dl.ID, func(d *data.Download) error { d.GID = "g2"; return nil })
	if err != nil {
		t.Fatalf("set gid: %v", err)
	}
	r.handle(downloader.Event{ID: dl.ID, GID: "g2", Type: downloader.EventFailed})
	got, _ = rpo.Get(context.Background(), dl.ID)
	if got.Status != data.StatusError {
		t.Fatalf("failed status = %v", got.Status)
	}
	if got.GID != "" {
		t.Fatalf("gid not cleared on failed: %q", got.GID)
	}
}

// TestHandleStartDoesNotOverrideStatus ensures that Start events do not
// resurrect downloads that have been paused or cancelled by the user before
// the downloader emitted the start signal.
func TestHandleStartDoesNotOverrideStatus(t *testing.T) {
	cases := []data.DownloadStatus{data.StatusCancelled, data.StatusPaused}
	for _, st := range cases {
		t.Run(string(st), func(t *testing.T) {
			rpo := repo.NewInMemoryDownloadRepo()
			dl := &data.Download{Source: "s", TargetPath: "t", Status: st, DesiredStatus: st}
			_, err := rpo.Add(context.Background(), dl)
			if err != nil {
				t.Fatalf("add: %v", err)
			}
			r := New(slog.New(slog.NewTextHandler(io.Discard, nil)), rpo, nil)

			r.handle(downloader.Event{ID: dl.ID, Type: downloader.EventStart})

			got, err := rpo.Get(context.Background(), dl.ID)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got.Status != st {
				t.Fatalf("start event overrode status: got %v want %v", got.Status, st)
			}
		})
	}
}

// TestHandleIgnoresStaleTerminalEvents ensures that terminal events with
// mismatched GIDs do not update repository state, while events that arrive
// before a GID is persisted are still processed.
func TestHandleIgnoresStaleTerminalEvents(t *testing.T) {
	t.Run("mismatched gid", func(t *testing.T) {
		rpo := repo.NewInMemoryDownloadRepo()
		dl := &data.Download{Source: "s", TargetPath: "t", Status: data.StatusActive, GID: "g"}
		_, err := rpo.Add(context.Background(), dl)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		r := New(slog.New(slog.NewTextHandler(io.Discard, nil)), rpo, nil)

		r.handle(downloader.Event{ID: dl.ID, GID: "other", Type: downloader.EventComplete})

		got, _ := rpo.Get(context.Background(), dl.ID)
		if got.Status != data.StatusActive {
			t.Fatalf("status changed: %v", got.Status)
		}
		if got.GID != "g" {
			t.Fatalf("gid changed: %q", got.GID)
		}
	})

	t.Run("missing repo gid", func(t *testing.T) {
		rpo := repo.NewInMemoryDownloadRepo()
		dl := &data.Download{Source: "s", TargetPath: "t", Status: data.StatusActive}
		_, err := rpo.Add(context.Background(), dl)
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		r := New(slog.New(slog.NewTextHandler(io.Discard, nil)), rpo, nil)

		r.handle(downloader.Event{ID: dl.ID, GID: "g", Type: downloader.EventFailed})

		got, _ := rpo.Get(context.Background(), dl.ID)
		if got.Status != data.StatusError {
			t.Fatalf("status not updated: %v", got.Status)
		}
		if got.GID != "" {
			t.Fatalf("gid not cleared: %q", got.GID)
		}
	})
}

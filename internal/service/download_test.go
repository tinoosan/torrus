package service

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/repo"
)

type stubDownloader struct {
    startFn  func(ctx context.Context, d *data.Download) (string, error)
    pauseFn  func(ctx context.Context, d *data.Download) error
    resumeFn func(ctx context.Context, d *data.Download) error
    cancelFn func(ctx context.Context, d *data.Download) error

    started   bool
    startCount int
    startedCh  chan struct{}
    paused    bool
    resumed   bool
    cancelled bool
}

func (s *stubDownloader) Start(ctx context.Context, d *data.Download) (string, error) {
    s.started = true
    s.startCount++
    if s.startedCh != nil {
        select { case s.startedCh <- struct{}{}: default: }
    }
    if s.startFn != nil {
        return s.startFn(ctx, d)
    }
    return "gid", nil
}
func (s *stubDownloader) Pause(ctx context.Context, d *data.Download) error {
    s.paused = true
    if s.pauseFn != nil {
        return s.pauseFn(ctx, d)
    }
    return nil
}
func (s *stubDownloader) Resume(ctx context.Context, d *data.Download) error {
    s.resumed = true
    if s.resumeFn != nil {
        return s.resumeFn(ctx, d)
    }
    return nil
}
func (s *stubDownloader) Cancel(ctx context.Context, d *data.Download) error {
	s.cancelled = true
	if s.cancelFn != nil {
		return s.cancelFn(ctx, d)
	}
	return nil
}

func TestUpdateDesiredStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("to Active starts and sets gid", func(t *testing.T) {
		r := repo.NewInMemoryDownloadRepo()
    d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t"})
    ch := make(chan struct{}, 2)
    dl := &stubDownloader{startFn: func(ctx context.Context, d *data.Download) (string, error) { return "g", nil }, startedCh: ch}
    svc := NewDownload(r, dl)

		got, err := svc.UpdateDesiredStatus(ctx, d.ID, data.StatusActive)
		if err != nil {
			t.Fatalf("UpdateDesiredStatus: %v", err)
		}
		if !dl.started {
			t.Fatalf("expected Start to be called")
		}
		if got.Status != data.StatusActive || got.GID != "g" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

    t.Run("to Paused pauses when gid", func(t *testing.T) {
		r := repo.NewInMemoryDownloadRepo()
		d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t", GID: "g", Status: data.StatusActive})
    dl := &stubDownloader{}
    svc := NewDownload(r, dl)

		got, err := svc.UpdateDesiredStatus(ctx, d.ID, data.StatusPaused)
		if err != nil {
			t.Fatalf("UpdateDesiredStatus: %v", err)
		}
		if !dl.paused {
			t.Fatalf("expected Pause to be called")
		}
		if got.Status != data.StatusPaused || got.GID != "g" {
			t.Fatalf("unexpected result: %#v", got)
		}
    })

    t.Run("to Resume resumes when gid and sets active", func(t *testing.T) {
        r := repo.NewInMemoryDownloadRepo()
        d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t", GID: "g", Status: data.StatusPaused})
        dl := &stubDownloader{}
        svc := NewDownload(r, dl)

        got, err := svc.UpdateDesiredStatus(ctx, d.ID, data.StatusResume)
        if err != nil {
            t.Fatalf("UpdateDesiredStatus: %v", err)
        }
        if !dl.resumed {
            t.Fatalf("expected Resume to be called")
        }
        if got.Status != data.StatusActive || got.GID != "g" {
            t.Fatalf("unexpected result: %#v", got)
        }
    })

	t.Run("to Cancelled cancels and clears gid", func(t *testing.T) {
		r := repo.NewInMemoryDownloadRepo()
		d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t", GID: "g", Status: data.StatusActive})
    dl := &stubDownloader{}
    svc := NewDownload(r, dl)

		got, err := svc.UpdateDesiredStatus(ctx, d.ID, data.StatusCancelled)
		if err != nil {
			t.Fatalf("UpdateDesiredStatus: %v", err)
		}
		if !dl.cancelled {
			t.Fatalf("expected Cancel to be called")
		}
		if got.Status != data.StatusCancelled || got.GID != "" {
			t.Fatalf("unexpected result: %#v", got)
		}
	})

	t.Run("downloader error sets failed", func(t *testing.T) {
		r := repo.NewInMemoryDownloadRepo()
    d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t"})
    dl := &stubDownloader{startFn: func(ctx context.Context, d *data.Download) (string, error) { return "", errors.New("boom") }}
    svc := NewDownload(r, dl)

		_, err := svc.UpdateDesiredStatus(ctx, d.ID, data.StatusActive)
		if err == nil {
			t.Fatalf("expected error")
		}
		got, _ := r.Get(ctx, d.ID)
		if got.Status != data.StatusError {
			t.Fatalf("status not failed: %s", got.Status)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		r := repo.NewInMemoryDownloadRepo()
    svc := NewDownload(r, &stubDownloader{})
		_, err := svc.UpdateDesiredStatus(ctx, 1, data.StatusQueued)
		if !errors.Is(err, data.ErrBadStatus) {
			t.Fatalf("expected ErrBadStatus, got %v", err)
		}
	})
}

func TestServiceAdd_Idempotent(t *testing.T) {
    ctx := context.Background()
    r := repo.NewInMemoryDownloadRepo()
    dl := &stubDownloader{}
    svc := NewDownload(r, dl)

    d := &data.Download{Source: "  s  ", TargetPath: " /x/y/../z "}
    got1, created1, err := svc.Add(ctx, d)
    if err != nil || !created1 {
        t.Fatalf("first add err=%v created=%v", err, created1)
    }
    if dl.startCount != 0 { // status is Queued by default
        t.Fatalf("unexpected start on queued: %d", dl.startCount)
    }

    ddup := &data.Download{Source: "s", TargetPath: "/x/z"}
    got2, created2, err := svc.Add(ctx, ddup)
    if err != nil || created2 {
        t.Fatalf("second add err=%v created=%v", err, created2)
    }
    if got1.ID != got2.ID {
        t.Fatalf("expected same id, got %d vs %d", got1.ID, got2.ID)
    }

    // Validation failures do not touch repo
    _, _, err = svc.Add(ctx, &data.Download{Source: "", TargetPath: "/x"})
    if !errors.Is(err, data.ErrInvalidSource) {
        t.Fatalf("expected ErrInvalidSource, got %v", err)
    }
}

func TestServiceAdd_ActiveNotRestartedOnDuplicate(t *testing.T) {
    ctx := context.Background()
    r := repo.NewInMemoryDownloadRepo()
    ch := make(chan struct{}, 2)
    dl := &stubDownloader{startFn: func(ctx context.Context, d *data.Download) (string, error) { return "g", nil }, startedCh: ch}
    svc := NewDownload(r, dl)

    // First add with DesiredStatus Active -> should start once
    first := &data.Download{Source: "s", TargetPath: "/t", DesiredStatus: data.StatusActive}
    _, created, err := svc.Add(ctx, first)
    if err != nil || !created {
        t.Fatalf("first add err=%v created=%v", err, created)
    }
    select {
    case <-ch:
        // ok
    case <-time.After(200 * time.Millisecond):
        t.Fatalf("downloader Start not called")
    }

    // Duplicate add with same pair: must not start again
    dup := &data.Download{Source: " s ", TargetPath: " /t/./"}
    _, created2, err := svc.Add(ctx, dup)
    if err != nil || created2 {
        t.Fatalf("dup add err=%v created=%v", err, created2)
    }
    // Ensure no additional start signal after a brief wait
    select {
    case <-ch:
        t.Fatalf("unexpected second Start on duplicate")
    case <-time.After(200 * time.Millisecond):
        // ok
    }
}

package service

import (
    "context"
    "errors"
    "testing"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloadcfg"
    "github.com/tinoosan/torrus/internal/repo"
)

type stubDownloader struct {
    startFn  func(ctx context.Context, d *data.Download, o downloadcfg.StartOptions) (string, error)
    pauseFn  func(ctx context.Context, d *data.Download) error
    resumeFn func(ctx context.Context, d *data.Download, o downloadcfg.StartOptions) error
    cancelFn func(ctx context.Context, d *data.Download) error

    started   bool
    paused    bool
    resumed   bool
    cancelled bool
}

func (s *stubDownloader) Start(ctx context.Context, d *data.Download, o downloadcfg.StartOptions) (string, error) {
    s.started = true
    if s.startFn != nil {
        return s.startFn(ctx, d, o)
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
func (s *stubDownloader) Resume(ctx context.Context, d *data.Download, o downloadcfg.StartOptions) error {
    s.resumed = true
    if s.resumeFn != nil {
        return s.resumeFn(ctx, d, o)
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
    dl := &stubDownloader{startFn: func(ctx context.Context, d *data.Download, _ downloadcfg.StartOptions) (string, error) { return "g", nil }}
    svc := NewDownload(r, dl, downloadcfg.CollisionError)

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
    svc := NewDownload(r, dl, downloadcfg.CollisionError)

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
        svc := NewDownload(r, dl, downloadcfg.CollisionError)

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
    svc := NewDownload(r, dl, downloadcfg.CollisionError)

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
    dl := &stubDownloader{startFn: func(ctx context.Context, d *data.Download, _ downloadcfg.StartOptions) (string, error) { return "", errors.New("boom") }}
    svc := NewDownload(r, dl, downloadcfg.CollisionError)

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
    svc := NewDownload(r, &stubDownloader{}, downloadcfg.CollisionError)
		_, err := svc.UpdateDesiredStatus(ctx, 1, data.StatusQueued)
		if !errors.Is(err, data.ErrBadStatus) {
			t.Fatalf("expected ErrBadStatus, got %v", err)
		}
	})
}

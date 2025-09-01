package service

import (
	"context"
	"errors"
	"strconv"
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
	deleteFn func(ctx context.Context, d *data.Download, deleteFiles bool) error

	started      bool
	startCount   int
	startedCh    chan struct{}
	paused       bool
	resumed      bool
	cancelled    bool
	deleted      bool
	deletedFiles bool
}

func (s *stubDownloader) Start(ctx context.Context, d *data.Download) (string, error) {
	s.started = true
	s.startCount++
	if s.startedCh != nil {
		select {
		case s.startedCh <- struct{}{}:
		default:
		}
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

func (s *stubDownloader) Delete(ctx context.Context, d *data.Download, deleteFiles bool) error {
	s.deleted = true
	s.deletedFiles = deleteFiles
	if s.deleteFn != nil {
		return s.deleteFn(ctx, d, deleteFiles)
	}
	return nil
}

// basicRepo implements repo.DownloadRepo but intentionally omits
// DownloadFinder methods to simulate a non-Extended repository.
type basicRepo struct {
	downloads data.Downloads
	nextID    int
}

func (r *basicRepo) List(ctx context.Context) (data.Downloads, error) {
	return r.downloads.Clone(), nil
}

func (r *basicRepo) Get(ctx context.Context, id string) (*data.Download, error) {
	for _, d := range r.downloads {
		if d.ID == id {
			return d.Clone(), nil
		}
	}
	return nil, data.ErrNotFound
}

func (r *basicRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	r.nextID++
	d.ID = strconv.Itoa(r.nextID - 1)
	r.downloads = append(r.downloads, d.Clone())
	return d.Clone(), nil
}

func (r *basicRepo) Update(ctx context.Context, id string, mutate func(*data.Download) error) (*data.Download, error) {
	for _, dl := range r.downloads {
		if dl.ID == id {
			if mutate != nil {
				if err := mutate(dl); err != nil {
					return nil, err
				}
			}
			return dl.Clone(), nil
		}
	}
	return nil, data.ErrNotFound
}

func (r *basicRepo) AddWithFingerprint(ctx context.Context, d *data.Download, fp string) (*data.Download, bool, error) {
	r.nextID++
	d.ID = strconv.Itoa(r.nextID - 1)
	r.downloads = append(r.downloads, d.Clone())
	return d.Clone(), true, nil
}

func (r *basicRepo) Delete(ctx context.Context, id string) error {
	for i, dl := range r.downloads {
		if dl.ID == id {
			r.downloads = append(r.downloads[:i], r.downloads[i+1:]...)
			return nil
		}
	}
	return data.ErrNotFound
}

func TestNewDownload_AllowsNonExtendedRepo(t *testing.T) {
	ctx := context.Background()
	r := &basicRepo{}
	svc := NewDownload(r, &stubDownloader{})
	if _, _, err := svc.Add(ctx, &data.Download{Source: "s", TargetPath: "t"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
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

	t.Run("resume without gid starts", func(t *testing.T) {
		r := repo.NewInMemoryDownloadRepo()
		d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t", Status: data.StatusPaused})
		ch := make(chan struct{}, 1)
		dl := &stubDownloader{startFn: func(ctx context.Context, d *data.Download) (string, error) {
			ch <- struct{}{}
			return "g2", nil
		}}
		svc := NewDownload(r, dl)

		got, err := svc.UpdateDesiredStatus(ctx, d.ID, data.StatusResume)
		if err != nil {
			t.Fatalf("UpdateDesiredStatus: %v", err)
		}
		select {
		case <-ch:
		default:
			t.Fatalf("expected Start to be called")
		}
		if dl.resumed {
			t.Fatalf("Resume should not be called without gid")
		}
		if got.Status != data.StatusActive || got.GID != "g2" {
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
		_, err := svc.UpdateDesiredStatus(ctx, "1", data.StatusQueued)
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
	if got1.DesiredStatus != data.StatusQueued || got1.Status != data.StatusQueued {
		t.Fatalf("defaults not applied: %#v", got1)
	}
	if got1.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt not set")
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
		t.Fatalf("expected same id, got %s vs %s", got1.ID, got2.ID)
	}

	// Validation failures do not touch repo
	_, _, err = svc.Add(ctx, &data.Download{Source: "", TargetPath: "/x"})
	if !errors.Is(err, data.ErrInvalidSource) {
		t.Fatalf("expected ErrInvalidSource, got %v", err)
	}
	_, _, err = svc.Add(ctx, &data.Download{Source: "http://ok", TargetPath: ""})
	if !errors.Is(err, data.ErrTargetPath) {
		t.Fatalf("expected ErrTargetPath, got %v", err)
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

func TestDownloadService_Delete(t *testing.T) {
	ctx := context.Background()
	r := repo.NewInMemoryDownloadRepo()
	d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t", GID: "g"})

	t.Run("delete record only", func(t *testing.T) {
		dlr := &stubDownloader{}
		svc := NewDownload(r, dlr)
		if err := svc.Delete(ctx, d.ID, false); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if !dlr.deleted || dlr.deletedFiles {
			t.Fatalf("expected Delete(false); got deleted=%v files=%v", dlr.deleted, dlr.deletedFiles)
		}
		if _, err := r.Get(ctx, d.ID); !errors.Is(err, data.ErrNotFound) {
			t.Fatalf("expected repo deletion, got %v", err)
		}
	})

	t.Run("delete with files", func(t *testing.T) {
		d2, _ := r.Add(ctx, &data.Download{Source: "s2", TargetPath: "t2", GID: "g2"})
		dlr := &stubDownloader{}
		svc := NewDownload(r, dlr)
		if err := svc.Delete(ctx, d2.ID, true); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if !dlr.deleted || !dlr.deletedFiles {
			t.Fatalf("expected Delete(true); got deleted=%v files=%v", dlr.deleted, dlr.deletedFiles)
		}
	})

	t.Run("delete without gid", func(t *testing.T) {
		d3, _ := r.Add(ctx, &data.Download{Source: "s3", TargetPath: "t3"})
		dlr := &stubDownloader{}
		svc := NewDownload(r, dlr)
		if err := svc.Delete(ctx, d3.ID, false); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if !dlr.deleted {
			t.Fatalf("expected Delete to be called even without gid")
		}
	})

	t.Run("delete files error", func(t *testing.T) {
		d4, _ := r.Add(ctx, &data.Download{Source: "s4", TargetPath: "t4"})
		dlr := &stubDownloader{deleteFn: func(ctx context.Context, d *data.Download, deleteFiles bool) error {
			return errors.New("boom")
		}}
		svc := NewDownload(r, dlr)
		if err := svc.Delete(ctx, d4.ID, true); err == nil {
			t.Fatalf("expected error")
		}
		if _, err := r.Get(ctx, d4.ID); err != nil {
			t.Fatalf("expected record retained, got %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		dlr := &stubDownloader{}
		svc := NewDownload(r, dlr)
		if err := svc.Delete(ctx, "999", false); !errors.Is(err, data.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

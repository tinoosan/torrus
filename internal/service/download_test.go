package service

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
	"github.com/tinoosan/torrus/internal/repo"
)

type mockDownloadRepo struct {
	listFn     func(ctx context.Context) (data.Downloads, error)
	getFn      func(ctx context.Context, id int) (*data.Download, error)
	addFn      func(ctx context.Context, d *data.Download) (*data.Download, error)
	updateFn   func(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)
	setFn      func(ctx context.Context, id int, status data.DownloadStatus) error
	setGIDFn   func(ctx context.Context, id int, gid string) error
	clearGIDFn func(ctx context.Context, id int) error

	listCalled     bool
	getCalled      bool
	addCalled      bool
	updateCalled   bool
	setCalled      bool
	setGIDCalled   bool
	clearGIDCalled bool

	setArgs struct {
		id     int
		gid    string
		status data.DownloadStatus
	}
}

var _ repo.DownloadRepo = (*mockDownloadRepo)(nil)

type mockDownloader struct {
	startCalled, pauseCalled, cancelCalled bool
	startFn                                func(ctx context.Context, d *data.Download) (string, error)
	pauseFn                                func(ctx context.Context, d *data.Download) error
	cancelFn                               func(ctx context.Context, d *data.Download) error
}

func (m *mockDownloader) Start(ctx context.Context, d *data.Download) (string, error) {
	m.startCalled = true
	if m.startFn != nil {
		return m.startFn(ctx, d)
	}
	return strconv.Itoa(d.ID), nil
}

func (m *mockDownloader) Pause(ctx context.Context, d *data.Download) error {
	m.pauseCalled = true
	if m.pauseFn != nil {
		return m.pauseFn(ctx, d)
	}
	return nil
}

func (m *mockDownloader) Cancel(ctx context.Context, d *data.Download) error {
	m.cancelCalled = true
	if m.cancelFn != nil {
		return m.cancelFn(ctx, d)
	}
	return nil
}

var _ downloader.Downloader = (*mockDownloader)(nil)

func (m *mockDownloadRepo) List(ctx context.Context) (data.Downloads, error) {
	m.listCalled = true
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockDownloadRepo) Get(ctx context.Context, id int) (*data.Download, error) {
	m.getCalled = true
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return nil, nil
}

func (m *mockDownloadRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	m.addCalled = true
	if m.addFn != nil {
		return m.addFn(ctx, d)
	}
	return nil, nil
}

func (m *mockDownloadRepo) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	m.updateCalled = true
	if m.updateFn != nil {
		return m.updateFn(ctx, id, status)
	}
	return nil, nil
}

func (m *mockDownloadRepo) SetStatus(ctx context.Context, id int, status data.DownloadStatus) error {
	m.setCalled = true
	m.setArgs.id = id
	m.setArgs.status = status
	if m.setFn != nil {
		return m.setFn(ctx, id, status)
	}
	return nil
}

func (m *mockDownloadRepo) SetGID(ctx context.Context, id int, gid string) error {
	m.setGIDCalled = true
	m.setArgs.id = id
	m.setArgs.gid = gid
	if m.setGIDFn != nil {
		return m.setGIDFn(ctx, id, gid)
	}
	return nil
}

func (m *mockDownloadRepo) ClearGID(ctx context.Context, id int) error {
	m.clearGIDCalled = true
	if m.clearGIDFn != nil {
		return m.clearGIDFn(ctx, id)
	}
	return nil
}

func TestDownloadService_List(t *testing.T) {
	ctx := context.Background()
	want := data.Downloads{{ID: 1}, {ID: 2}}
	m := &mockDownloadRepo{
		listFn: func(ctx context.Context) (data.Downloads, error) {
			return want, nil
		},
	}
	svc := NewDownload(m, &mockDownloader{})
	got, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List mismatch: got %#v want %#v", got, want)
	}
	if !m.listCalled {
		t.Fatalf("expected repo List to be called")
	}
}

func TestDownloadService_Get(t *testing.T) {
	ctx := context.Background()
	t.Run("found", func(t *testing.T) {
		d := &data.Download{ID: 5}
		m := &mockDownloadRepo{
			getFn: func(ctx context.Context, id int) (*data.Download, error) {
				if id != d.ID {
					t.Fatalf("expected id %d got %d", d.ID, id)
				}
				return d, nil
			},
		}
		svc := NewDownload(m, &mockDownloader{})
		got, err := svc.Get(ctx, d.ID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}
		if !reflect.DeepEqual(got, d) {
			t.Fatalf("Get mismatch: got %#v want %#v", got, d)
		}
		if !m.getCalled {
			t.Fatalf("expected repo Get to be called")
		}
	})

	t.Run("not found", func(t *testing.T) {
		m := &mockDownloadRepo{
			getFn: func(ctx context.Context, id int) (*data.Download, error) {
				return nil, data.ErrNotFound
			},
		}
		svc := NewDownload(m, &mockDownloader{})
		got, err := svc.Get(ctx, 1)
		if !errors.Is(err, data.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil download, got %#v", got)
		}
		if !m.getCalled {
			t.Fatalf("expected repo Get to be called")
		}
	})
}

func TestDownloadService_Add(t *testing.T) {
	ctx := context.Background()

	t.Run("defaults and delegates", func(t *testing.T) {
		var received *data.Download
		m := &mockDownloadRepo{
			addFn: func(ctx context.Context, d *data.Download) (*data.Download, error) {
				received = d
				d.ID = 1
				return d, nil
			},
		}
		svc := NewDownload(m, &mockDownloader{})
		input := &data.Download{Source: "s", TargetPath: "t"}
		got, err := svc.Add(ctx, input)
		if err != nil {
			t.Fatalf("Add returned error: %v", err)
		}
		if !m.addCalled {
			t.Fatalf("expected repo Add to be called")
		}
		if received == nil {
			t.Fatalf("repo Add did not receive download")
		}
		if received.CreatedAt.IsZero() {
			t.Fatalf("CreatedAt was not set")
		}
		if received.Status != data.StatusQueued {
			t.Fatalf("Status not defaulted: %s", received.Status)
		}
		if received.DesiredStatus != data.StatusQueued {
			t.Fatalf("DesiredStatus not defaulted: %s", received.DesiredStatus)
		}
		if got.ID != 1 {
			t.Fatalf("unexpected ID %d", got.ID)
		}
	})

	t.Run("missing source", func(t *testing.T) {
		m := &mockDownloadRepo{}
		svc := NewDownload(m, &mockDownloader{})
		_, err := svc.Add(ctx, &data.Download{TargetPath: "t"})
		if !errors.Is(err, data.ErrInvalidSource) {
			t.Fatalf("expected ErrInvalidSource got %v", err)
		}
		if m.addCalled {
			t.Fatalf("repo Add should not be called")
		}
	})

	t.Run("missing target path", func(t *testing.T) {
		m := &mockDownloadRepo{}
		svc := NewDownload(m, &mockDownloader{})
		_, err := svc.Add(ctx, &data.Download{Source: "s"})
		if !errors.Is(err, data.ErrTargetPath) {
			t.Fatalf("expected ErrTargetPath got %v", err)
		}
		if m.addCalled {
			t.Fatalf("repo Add should not be called")
		}
	})
}

func TestDownloadService_UpdateDesiredStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("Active \u2192 calls Start and sets status=Active", func(t *testing.T) {
		getCalls := 0
		mRepo := &mockDownloadRepo{
			getFn: func(ctx context.Context, id int) (*data.Download, error) {
				getCalls++
				if getCalls == 1 {
					return &data.Download{ID: id}, nil
				}
				return &data.Download{ID: id, DesiredStatus: data.StatusActive, Status: data.StatusActive}, nil
			},
			updateFn: func(ctx context.Context, id int, s data.DownloadStatus) (*data.Download, error) {
				if s != data.StatusActive {
					t.Fatalf("expected desired Active, got %s", s)
				}
				return &data.Download{ID: 42, DesiredStatus: s, Status: data.StatusQueued}, nil
			},
			setFn: func(ctx context.Context, id int, s data.DownloadStatus) error {
				if id != 42 {
					t.Fatalf("SetStatus id mismatch: %d", id)
				}
				if s != data.StatusActive {
					t.Fatalf("expected final status Active, got %s", s)
				}
				return nil
			},
		}
		mDL := &mockDownloader{}

		svc := NewDownload(mRepo, mDL)
		got, err := svc.UpdateDesiredStatus(ctx, 42, data.StatusActive)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !mDL.startCalled {
			t.Fatalf("expected Start to be called")
		}
		if !mRepo.setCalled {
			t.Fatalf("expected SetStatus to be called")
		}
		if got.DesiredStatus != data.StatusActive {
			t.Fatalf("got desired %s", got.DesiredStatus)
		}
	})

	t.Run("Paused \u2192 calls Pause and sets status=Paused", func(t *testing.T) {
		getCalls := 0
		mRepo := &mockDownloadRepo{
			getFn: func(ctx context.Context, id int) (*data.Download, error) {
				getCalls++
				if getCalls == 1 {
					return &data.Download{ID: id, GID: "gid"}, nil
				}
				return &data.Download{ID: id, DesiredStatus: data.StatusPaused, Status: data.StatusPaused, GID: "gid"}, nil
			},
			updateFn: func(ctx context.Context, id int, s data.DownloadStatus) (*data.Download, error) {
				if s != data.StatusPaused {
					t.Fatalf("expected desired Paused, got %s", s)
				}
				return &data.Download{ID: 7, DesiredStatus: s, Status: data.StatusQueued}, nil
			},
			setFn: func(ctx context.Context, id int, s data.DownloadStatus) error {
				if id != 7 {
					t.Fatalf("SetStatus id mismatch: %d", id)
				}
				if s != data.StatusPaused {
					t.Fatalf("expected final status Paused, got %s", s)
				}
				return nil
			},
		}
		mDL := &mockDownloader{}

		svc := NewDownload(mRepo, mDL)
		got, err := svc.UpdateDesiredStatus(ctx, 7, data.StatusPaused)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !mDL.pauseCalled {
			t.Fatalf("expected Pause to be called")
		}
		if !mRepo.setCalled {
			t.Fatalf("expected SetStatus to be called")
		}
		if got.DesiredStatus != data.StatusPaused {
			t.Fatalf("got desired %s", got.DesiredStatus)
		}
	})

	t.Run("Cancelled \u2192 calls Cancel and sets status=Cancelled", func(t *testing.T) {
		getCalls := 0
		mRepo := &mockDownloadRepo{
			getFn: func(ctx context.Context, id int) (*data.Download, error) {
				getCalls++
				if getCalls == 1 {
					return &data.Download{ID: id, GID: "gid"}, nil
				}
				return &data.Download{ID: id, DesiredStatus: data.StatusCancelled, Status: data.StatusCancelled}, nil
			},
			updateFn: func(ctx context.Context, id int, s data.DownloadStatus) (*data.Download, error) {
				if s != data.StatusCancelled {
					t.Fatalf("expected desired Cancelled, got %s", s)
				}
				return &data.Download{ID: 3, DesiredStatus: s, Status: data.StatusQueued}, nil
			},
			setFn: func(ctx context.Context, id int, s data.DownloadStatus) error {
				if id != 3 {
					t.Fatalf("SetStatus id mismatch: %d", id)
				}
				if s != data.StatusCancelled {
					t.Fatalf("expected final status Cancelled, got %s", s)
				}
				return nil
			},
		}
		mDL := &mockDownloader{}

		svc := NewDownload(mRepo, mDL)
		got, err := svc.UpdateDesiredStatus(ctx, 3, data.StatusCancelled)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !mDL.cancelCalled {
			t.Fatalf("expected Cancel to be called")
		}
		if !mRepo.setCalled {
			t.Fatalf("expected SetStatus to be called")
		}
		if got.DesiredStatus != data.StatusCancelled {
			t.Fatalf("got desired %s", got.DesiredStatus)
		}
	})

	t.Run("Downloader error \u2192 sets status=Failed", func(t *testing.T) {
		mRepo := &mockDownloadRepo{
			getFn: func(ctx context.Context, id int) (*data.Download, error) {
				return &data.Download{ID: id}, nil
			},
			updateFn: func(ctx context.Context, id int, s data.DownloadStatus) (*data.Download, error) {
				return &data.Download{ID: 99, DesiredStatus: s, Status: data.StatusQueued}, nil
			},
			setFn: func(ctx context.Context, id int, s data.DownloadStatus) error {
				if s != data.StatusError {
					t.Fatalf("expected final status Failed, got %s", s)
				}
				return nil
			},
		}
		mDL := &mockDownloader{
			startFn: func(ctx context.Context, d *data.Download) (string, error) { return "", errors.New("boom") },
		}

		svc := NewDownload(mRepo, mDL)
		got, err := svc.UpdateDesiredStatus(ctx, 99, data.StatusActive)
		if err == nil {
			t.Fatalf("expected error from downloader")
		}
		if !mDL.startCalled {
			t.Fatalf("expected Start to be called")
		}
		if !mRepo.setCalled {
			t.Fatalf("expected SetStatus(Failed) to be called")
		}
		if got != nil {
			t.Fatalf("expected nil result on failure")
		}
	})

	invalid := []data.DownloadStatus{data.StatusQueued, data.StatusComplete, data.StatusError, "bogus"}
	for _, st := range invalid {
		t.Run("invalid "+string(st), func(t *testing.T) {
			mRepo := &mockDownloadRepo{}
			svc := NewDownload(mRepo, &mockDownloader{})
			_, err := svc.UpdateDesiredStatus(ctx, 1, st)
			if !errors.Is(err, data.ErrBadStatus) {
				t.Fatalf("expected ErrBadStatus got %v", err)
			}
			if mRepo.updateCalled {
				t.Fatalf("repo should not be called for invalid status")
			}
		})
	}
}

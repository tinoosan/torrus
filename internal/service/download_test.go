package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/repo"
)

type mockDownloadRepo struct {
	listFn   func(ctx context.Context) (data.Downloads, error)
	getFn    func(ctx context.Context, id int) (*data.Download, error)
	addFn    func(ctx context.Context, d *data.Download) (*data.Download, error)
	updateFn func(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)

	listCalled   bool
	getCalled    bool
	addCalled    bool
	updateCalled bool
}

var _ repo.DownloadRepo = (*mockDownloadRepo)(nil)

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

func TestDownloadService_List(t *testing.T) {
	ctx := context.Background()
	want := data.Downloads{{ID: 1}, {ID: 2}}
	m := &mockDownloadRepo{
		listFn: func(ctx context.Context) (data.Downloads, error) {
			return want, nil
		},
	}
	svc := NewDownload(m)
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
		svc := NewDownload(m)
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
		svc := NewDownload(m)
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
		svc := NewDownload(m)
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
		svc := NewDownload(m)
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
		svc := NewDownload(m)
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
	valid := []data.DownloadStatus{data.StatusActive, data.StatusPaused, data.StatusCancelled}
	for _, st := range valid {
		t.Run("valid "+string(st), func(t *testing.T) {
			m := &mockDownloadRepo{
				updateFn: func(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
					if status != st {
						t.Fatalf("expected status %s got %s", st, status)
					}
					return &data.Download{ID: id, DesiredStatus: status}, nil
				},
			}
			svc := NewDownload(m)
			got, err := svc.UpdateDesiredStatus(ctx, 1, st)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !m.updateCalled {
				t.Fatalf("expected repo UpdateDesiredStatus to be called")
			}
			if got.DesiredStatus != st {
				t.Fatalf("expected desired status %s got %s", st, got.DesiredStatus)
			}
		})
	}

	invalid := []data.DownloadStatus{data.StatusQueued, data.StatusComplete, data.StatusError, "bogus"}
	for _, st := range invalid {
		t.Run("invalid "+string(st), func(t *testing.T) {
			m := &mockDownloadRepo{}
			svc := NewDownload(m)
			_, err := svc.UpdateDesiredStatus(ctx, 1, st)
			if !errors.Is(err, data.ErrBadStatus) {
				t.Fatalf("expected ErrBadStatus got %v", err)
			}
			if m.updateCalled {
				t.Fatalf("repo should not be called for invalid status")
			}
		})
	}
}

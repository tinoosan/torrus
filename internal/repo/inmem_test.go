package repo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/tinoosan/torrus/internal/data"
)

func TestInMemoryDownloadRepo_Add(t *testing.T) {
	repo := NewInMemoryDownloadRepo()
	ctx := context.Background()

	d1, err := repo.Add(ctx, &data.Download{Source: "s1", TargetPath: "t1"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if d1.ID != 1 {
		t.Fatalf("expected ID 1 got %d", d1.ID)
	}

	d2, err := repo.Add(ctx, &data.Download{Source: "s2", TargetPath: "t2"})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if d2.ID != 2 {
		t.Fatalf("expected ID 2 got %d", d2.ID)
	}
}

func TestInMemoryDownloadRepo_List(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryDownloadRepo()

	// empty repo
	list, _ := repo.List(ctx)
	if got := len(list); got != 0 {
		t.Fatalf("expected empty list, got %d", got)
	}

	d1, _ := repo.Add(ctx, &data.Download{Source: "s1", TargetPath: "t1"})
	_, _ = repo.Add(ctx, &data.Download{Source: "s2", TargetPath: "t2"})

	list1, _ := repo.List(ctx)
	if len(list1) != 2 {
		t.Fatalf("expected 2 downloads, got %d", len(list1))
	}

	// modify returned slice
	list1[0] = &data.Download{ID: 99}
	list1 = append(list1, &data.Download{ID: 100})

	list2, _ := repo.List(ctx)
	if len(list2) != 2 {
		t.Fatalf("expected 2 downloads after modification, got %d", len(list2))
	}
	if list2[0].ID != d1.ID {
		t.Fatalf("expected first ID %d got %d", d1.ID, list2[0].ID)
	}
}

func TestInMemoryDownloadRepo_Get(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryDownloadRepo()
	want, _ := repo.Add(ctx, &data.Download{Source: "s1", TargetPath: "t1"})

	tests := []struct {
		name    string
		repo    *InMemoryDownloadRepo
		id      int
		want    *data.Download
		wantErr error
	}{
		{"exists", repo, want.ID, want, nil},
		{"not found", repo, 999, nil, data.ErrNotFound},
		{"empty repo", NewInMemoryDownloadRepo(), 1, nil, data.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.repo.Get(ctx, tt.id)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil {
				if !reflect.DeepEqual(*got, *tt.want) {
					t.Fatalf("mismatch:\n got:  %#v\n want: %#v", got, tt.want)
				}
			}
		})
	}
}

func TestInMemoryDownloadRepo_UpdateDesiredStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("valid", func(t *testing.T) {
		repo := NewInMemoryDownloadRepo()
		d, _ := repo.Add(ctx, &data.Download{Source: "s", TargetPath: "t"})
		updated, err := repo.UpdateDesiredStatus(ctx, d.ID, data.StatusPaused)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.DesiredStatus != data.StatusPaused {
			t.Fatalf("expected desired status %s got %s", data.StatusPaused, updated.DesiredStatus)
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		repo := NewInMemoryDownloadRepo()
		if _, err := repo.UpdateDesiredStatus(ctx, 123, data.StatusPaused); !errors.Is(err, data.ErrNotFound) {
			t.Fatalf("expected ErrNotFound got %v", err)
		}
	})
}

func TestInMemoryDownloadRepo_Concurrency(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryDownloadRepo()
	const n = 50
	var wg sync.WaitGroup

	// reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			repo.List(ctx)
			repo.Get(ctx, i)
		}
	}()

	// concurrent writers
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := repo.Add(ctx, &data.Download{Source: fmt.Sprintf("s%d", i), TargetPath: "t"}); err != nil {
				t.Errorf("Add error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	list, _ := repo.List(ctx)

	if got := len(list); got != n {
		t.Fatalf("expected %d downloads, got %d", n, got)
	}
}

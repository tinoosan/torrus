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

func TestInMemoryDownloadRepo_AddListGet(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()

	d1, err := r.Add(ctx, &data.Download{Source: "s1", TargetPath: "t1"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if d1.ID != 1 {
		t.Fatalf("first id = %d", d1.ID)
	}

	d2, _ := r.Add(ctx, &data.Download{Source: "s2", TargetPath: "t2"})
	if d2.ID != 2 {
		t.Fatalf("second id = %d", d2.ID)
	}

	list, err := r.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %v len %d err %v", list, len(list), err)
	}

	// ensure clones returned
	list[0].ID = 99
	again, _ := r.List(ctx)
	if again[0].ID != d1.ID {
		t.Fatalf("repo mutated via clone")
	}

	got, err := r.Get(ctx, d1.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reflect.DeepEqual(got, d1) {
		t.Fatalf("Get mismatch")
	}

	_, err = r.Get(ctx, 999)
	if !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("expected ErrNotFound")
	}
}

func TestInMemoryDownloadRepo_Update(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()
	d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t"})

	t.Run("noop", func(t *testing.T) {
		before, _ := r.Get(ctx, d.ID)
		got, err := r.Update(ctx, d.ID, func(dl *data.Download) error { return nil })
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		after, _ := r.Get(ctx, d.ID)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("expected no change")
		}
		got.GID = "mut"
		again, _ := r.Get(ctx, d.ID)
		if again.GID == "mut" {
			t.Fatalf("clone not deep")
		}
	})

	t.Run("single fields", func(t *testing.T) {
		cases := []struct {
			name   string
			mutate func(*data.Download)
			check  func(*data.Download) bool
		}{
			{"desired", func(dl *data.Download) { dl.DesiredStatus = data.StatusPaused }, func(dl *data.Download) bool { return dl.DesiredStatus == data.StatusPaused }},
			{"status", func(dl *data.Download) { dl.Status = data.StatusComplete }, func(dl *data.Download) bool { return dl.Status == data.StatusComplete }},
			{"gid", func(dl *data.Download) { dl.GID = "G1" }, func(dl *data.Download) bool { return dl.GID == "G1" }},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := r.Update(ctx, d.ID, func(dl *data.Download) error { tc.mutate(dl); return nil })
				if err != nil {
					t.Fatalf("Update: %v", err)
				}
				if !tc.check(got) {
					t.Fatalf("field not updated: %#v", got)
				}
			})
		}
	})

	t.Run("multi", func(t *testing.T) {
		got, err := r.Update(ctx, d.ID, func(dl *data.Download) error {
			dl.DesiredStatus = data.StatusActive
			dl.Status = data.StatusActive
			dl.GID = "GG"
			return nil
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if got.DesiredStatus != data.StatusActive || got.Status != data.StatusActive || got.GID != "GG" {
			t.Fatalf("multi update failed: %#v", got)
		}
	})

	t.Run("clear gid", func(t *testing.T) {
		_, _ = r.Update(ctx, d.ID, func(dl *data.Download) error { dl.GID = "abc"; return nil })
		got, err := r.Update(ctx, d.ID, func(dl *data.Download) error { dl.GID = ""; return nil })
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if got.GID != "" {
			t.Fatalf("gid not cleared")
		}
	})
}

func TestInMemoryDownloadRepo_Update_Concurrent(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()
	d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t"})

	gids := []string{"G1", "G2", "G3", "G4", "G5"}
	var wg sync.WaitGroup
	for _, g := range gids {
		wg.Add(1)
		go func(g string) {
			defer wg.Done()
			got, err := r.Update(ctx, d.ID, func(dl *data.Download) error { dl.GID = g; return nil })
			if err != nil {
				t.Errorf("Update: %v", err)
				return
			}
			got.GID = "mut"
			res, _ := r.Get(ctx, d.ID)
			if res.GID == "mut" {
				t.Errorf("clone not deep")
			}
		}(g)
	}
	wg.Wait()

	got, err := r.Get(ctx, d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	found := false
	for _, g := range gids {
		if got.GID == g {
			found = true
		}
	}
	if !found {
		t.Fatalf("final gid %q not in %v", got.GID, gids)
	}
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
			_, err := repo.List(ctx)
			if err != nil {
				t.Errorf("List error: %v", err)
			}
			_, err = repo.Get(ctx, i)
			if err != nil && !errors.Is(err, data.ErrNotFound) {
				t.Errorf("Get error: %v", err)
			}
		}
	}()

	// concurrent writers
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.Add(ctx, &data.Download{Source: fmt.Sprintf("s%d", i), TargetPath: "t"})
			if err != nil {
				t.Errorf("Add error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}

	if got := len(list); got != n {
		t.Fatalf("expected %d downloads, got %d", n, got)
	}
}

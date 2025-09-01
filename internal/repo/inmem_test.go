package repo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/tinoosan/torrus/internal/data"
)

func TestInMemoryDownloadRepo_AddListGet(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()

	d1, err := r.Add(ctx, &data.Download{Source: "s1", TargetPath: "t1"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := uuid.Parse(d1.ID); err != nil {
		t.Fatalf("invalid uuid: %v", err)
	}

	d2, _ := r.Add(ctx, &data.Download{Source: "s2", TargetPath: "t2"})
	if d1.ID == d2.ID {
		t.Fatalf("ids not unique")
	}

	list, err := r.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %v len %d err %v", list, len(list), err)
	}

	// ensure clones returned
	list[0].ID = "mut"
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

	_, err = r.Get(ctx, uuid.NewString())
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
			if _, err := repo.List(ctx); err != nil {
				t.Errorf("List error: %v", err)
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

func TestInMemoryDownloadRepo_Delete(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()
	d, _ := r.Add(ctx, &data.Download{Source: "s", TargetPath: "t"})

	if err := r.Delete(ctx, d.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := r.Get(ctx, d.ID); !errors.Is(err, data.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAddWithFingerprint_IdempotentAndGetByFingerprint(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()

	// First insert
	d := &data.Download{Source: " s ", TargetPath: " t "}
	got, created, err := r.AddWithFingerprint(ctx, d, "fp1")
	if err != nil || !created {
		t.Fatalf("expected created=true got err=%v created=%v", err, created)
	}
	if _, err := uuid.Parse(got.ID); err != nil {
		t.Fatalf("id not assigned: %v", err)
	}

	// Second insert with same fingerprint
	d2 := &data.Download{Source: " s ", TargetPath: " t "}
	got2, created2, err := r.AddWithFingerprint(ctx, d2, "fp1")
	if err != nil || created2 {
		t.Fatalf("expected created=false got err=%v created=%v", err, created2)
	}
	if got2.ID != got.ID {
		t.Fatalf("expected same id got %s vs %s", got2.ID, got.ID)
	}

	// Lookup by fingerprint
	byfp, err := r.GetByFingerprint(ctx, "fp1")
	if err != nil || byfp.ID != got.ID {
		t.Fatalf("GetByFingerprint mismatch: %#v err=%v", byfp, err)
	}
}

func TestAddWithFingerprint_ConcurrentSingleCreate(t *testing.T) {
	ctx := context.Background()
	r := NewInMemoryDownloadRepo()

	const gor = 50
	var wg sync.WaitGroup
	ids := make(chan string, gor)
	for i := 0; i < gor; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := &data.Download{Source: "s", TargetPath: "t"}
			got, _, err := r.AddWithFingerprint(ctx, d, "samefp")
			if err != nil {
				t.Errorf("AddWithFingerprint: %v", err)
				return
			}
			ids <- got.ID
		}()
	}
	wg.Wait()
	close(ids)

	first := ""
	for id := range ids {
		if first == "" {
			first = id
		} else if id != first {
			t.Fatalf("saw different ids: %s vs %s", id, first)
		}
	}

	list, _ := r.List(ctx)
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
}

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
	"github.com/tinoosan/torrus/internal/fp"
	"github.com/tinoosan/torrus/internal/repo"
)

// Download provides high-level operations for managing downloads.
type Download interface {
	List(ctx context.Context) (data.Downloads, error)
	Get(ctx context.Context, id int) (*data.Download, error)
	// Add inserts a new download or returns an existing one if it already
	// exists (idempotent). The returned 'created' flag indicates whether a new
	// row was created (true) or an existing one was returned (false).
	Add(ctx context.Context, d *data.Download) (*data.Download, bool, error)
	UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)
}

var (
	// AllowedStatuses enumerates statuses that callers may request.
	AllowedStatuses = map[data.DownloadStatus]bool{
		data.StatusActive:    true,
		data.StatusResume:    true,
		data.StatusPaused:    true,
		data.StatusCancelled: true,
	}
)

// download implements the Download service.
type download struct {
	repo repo.ExtendedRepo
	dlr  downloader.Downloader
}

// NewDownload constructs a Download service backed by the given repository and downloader.
func NewDownload(r repo.DownloadRepo, dlr downloader.Downloader) Download {
	// If the repository does not implement ExtendedRepo, wrap it in a minimal
	// adapter that satisfies the interface but disables fingerprint lookups.
	ext, ok := r.(repo.ExtendedRepo)
	if !ok {
		ext = &extendedRepoAdapter{DownloadRepo: r}
	}
	return &download{repo: ext, dlr: dlr}
}

// extendedRepoAdapter bridges a DownloadRepo that lacks DownloadFinder
// methods into an ExtendedRepo. GetByFingerprint always reports not found,
// so callers lose idempotency when using this fallback.
type extendedRepoAdapter struct {
	repo.DownloadRepo
}

func (a *extendedRepoAdapter) GetByFingerprint(ctx context.Context, fp string) (*data.Download, error) {
	return nil, data.ErrNotFound
}

// List returns all downloads from the repository.
func (ds *download) List(ctx context.Context) (data.Downloads, error) {
	return ds.repo.List(ctx)
}

// Get retrieves a download by its ID.
func (ds *download) Get(ctx context.Context, id int) (*data.Download, error) {
	return ds.repo.Get(ctx, id)
}

// Add validates and persists a new download request.
func (ds *download) Add(ctx context.Context, d *data.Download) (*data.Download, bool, error) {
	if strings.TrimSpace(d.Source) == "" {
		return nil, false, data.ErrInvalidSource
	}
	if strings.TrimSpace(d.TargetPath) == "" {
		return nil, false, data.ErrTargetPath
	}

	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}

	switch d.DesiredStatus {
	case "", data.StatusQueued:
		d.DesiredStatus = data.StatusQueued
		d.Status = data.StatusQueued
	case data.StatusActive:
		d.Status = data.StatusActive
	case data.StatusPaused:
		d.Status = data.StatusPaused
	case data.StatusCancelled:
		return nil, false, data.ErrBadStatus
	default:
		return nil, false, data.ErrBadStatus
	}

	// Compute idempotency fingerprint and insert or return existing.
	fp := fp.Fingerprint(d.Source, d.TargetPath)
	saved, created, err := ds.repo.AddWithFingerprint(ctx, d, fp)
	if err != nil {
		return nil, false, err
	}

	// Only trigger a new start when this call actually created the download.
	// Idempotent hits (created=false) must not re-start already active items.
	if saved.Status == data.StatusActive && created {
		go func(d *data.Download) {
			gid, derr := ds.dlr.Start(context.Background(), d)
			if derr != nil {
				_, _ = ds.repo.Update(context.Background(), d.ID, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return
			}
			_, err := ds.repo.Update(context.Background(), d.ID, func(dl *data.Download) error {
				dl.GID = gid
				return nil
			})
			if err != nil {
				_, _ = ds.repo.Update(context.Background(), d.ID, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return
			}
			d.GID = gid
		}(saved)
	}
	return saved, created, nil
}

// UpdateDesiredStatus changes the desired state of a download and performs the
// necessary side effects to reach it.
func (ds *download) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	// Guard invalid desired statuses up front (service-level policy).
	switch status {
	case data.StatusActive, data.StatusResume, data.StatusPaused, data.StatusCancelled:
	default:
		return nil, data.ErrBadStatus
	}

	// Always fetch the latest state first.
	cur, err := ds.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Persist desiredStatus (so callers see intent even if the actual action fails).
	_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
		dl.DesiredStatus = status
		return nil
	})
	if err != nil {
		return nil, err
	}

	switch status {
	case data.StatusActive:
		// Start if we don't have a GID yet (fresh start).
		// If we *do* have a GID, keep it simple for MVP: just set Status=Active
		// and return (future: introduce Resume in downloader and call it here).
		if cur.GID == "" {
			gid, derr := ds.dlr.Start(ctx, cur) // uses Source + TargetPath from cur
			if derr != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				if errors.Is(derr, data.ErrConflict) {
					return nil, data.ErrConflict
				}
				return nil, derr
			}
			_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
				dl.GID = gid
				return nil
			})
			if err != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, err
			}
		}
		_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.Status = data.StatusActive
			return nil
		})
		if err != nil {
			return nil, err
		}

	case data.StatusResume:
		// If we have a GID, call Resume (unpause). If not, fall back to Start.
		if cur.GID != "" {
			derr := ds.dlr.Resume(ctx, cur)
			if derr != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				if errors.Is(derr, data.ErrConflict) {
					return nil, data.ErrConflict
				}
				return nil, derr
			}
		} else {
			gid, derr := ds.dlr.Start(ctx, cur)
			if derr != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				if errors.Is(derr, data.ErrConflict) {
					return nil, data.ErrConflict
				}
				return nil, derr
			}
			_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
				dl.GID = gid
				return nil
			})
			if err != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, err
			}
		}
		_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.Status = data.StatusActive
			return nil
		})
		if err != nil {
			return nil, err
		}

	case data.StatusPaused:
		// If no GID, nothing to pause (treat as success: desired=Paused, status=Paused).
		if cur.GID != "" {
			derr := ds.dlr.Pause(ctx, cur)
			if derr != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, derr
			}
		}
		_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.Status = data.StatusPaused
			return nil
		})
		if err != nil {
			return nil, err
		}

	case data.StatusCancelled:
		// Try to cancel if we have a GID; treat "not found" as success for idempotency.
		if cur.GID != "" {
			derr := ds.dlr.Cancel(ctx, cur)
			if derr != nil && !isDownloaderNotFound(derr) {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, derr
			}
		}
		_, err = ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.GID = ""
			dl.Status = data.StatusCancelled
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Return the latest snapshot.
	return ds.repo.Get(ctx, id)
}

func isDownloaderNotFound(err error) bool {
	return errors.Is(err, downloader.ErrNotFound)
}

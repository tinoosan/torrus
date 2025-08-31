package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
	"github.com/tinoosan/torrus/internal/repo"
)

// Download provides high-level operations for managing downloads.
type Download interface {
	List(ctx context.Context) (data.Downloads, error)
	Get(ctx context.Context, id int) (*data.Download, error)
	Add(ctx context.Context, d *data.Download) (*data.Download, error)
	UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)
}

var (
	// AllowedStatuses enumerates statuses that callers may request.
	AllowedStatuses = map[data.DownloadStatus]bool{
		data.StatusActive:    true,
		data.StatusPaused:    true,
		data.StatusCancelled: true,
	}
)

// download implements the Download service.
type download struct {
	repo repo.DownloadRepo
	dlr  downloader.Downloader
}

// NewDownload constructs a Download service backed by the given repository and downloader.
func NewDownload(repo repo.DownloadRepo, dlr downloader.Downloader) Download {
	return &download{
		repo: repo,
		dlr:  dlr,
	}
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
func (ds *download) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	if strings.TrimSpace(d.Source) == "" {
		return nil, data.ErrInvalidSource
	}
	if strings.TrimSpace(d.TargetPath) == "" {
		return nil, data.ErrTargetPath
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
		return nil, data.ErrBadStatus
	default:
		return nil, data.ErrBadStatus
	}

	saved, err := ds.repo.Add(ctx, d)
	if err != nil {
		return nil, err
	}

	if saved.Status == data.StatusActive {
		go func(d *data.Download) {
			gid, derr := ds.dlr.Start(context.Background(), d)
			if derr != nil {
				_, _ = ds.repo.Update(context.Background(), d.ID, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return
			}
			if _, err := ds.repo.Update(context.Background(), d.ID, func(dl *data.Download) error {
				dl.GID = gid
				return nil
			}); err != nil {
				_, _ = ds.repo.Update(context.Background(), d.ID, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return
			}
			d.GID = gid
		}(saved)
	}
	return saved, nil
}

// UpdateDesiredStatus changes the desired state of a download and performs the
// necessary side effects to reach it.
func (ds *download) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	// Guard invalid desired statuses up front (service-level policy).
	switch status {
	case data.StatusActive, data.StatusPaused, data.StatusCancelled:
	default:
		return nil, data.ErrBadStatus
	}

	// Always fetch the latest state first.
	cur, err := ds.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Persist desiredStatus (so callers see intent even if the actual action fails).
	if _, err := ds.repo.Update(ctx, id, func(dl *data.Download) error {
		dl.DesiredStatus = status
		return nil
	}); err != nil {
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
				return nil, derr
			}
			if _, err := ds.repo.Update(ctx, id, func(dl *data.Download) error {
				dl.GID = gid
				return nil
			}); err != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, err
			}
		}
		if _, err := ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.Status = data.StatusActive
			return nil
		}); err != nil {
			return nil, err
		}

	case data.StatusPaused:
		// If no GID, nothing to pause (treat as success: desired=Paused, status=Paused).
		if cur.GID != "" {
			if derr := ds.dlr.Pause(ctx, cur); derr != nil {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, derr
			}
		}
		if _, err := ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.Status = data.StatusPaused
			return nil
		}); err != nil {
			return nil, err
		}

	case data.StatusCancelled:
		// Try to cancel if we have a GID; treat "not found" as success for idempotency.
		if cur.GID != "" {
			if derr := ds.dlr.Cancel(ctx, cur); derr != nil && !isDownloaderNotFound(derr) {
				_, _ = ds.repo.Update(ctx, id, func(dl *data.Download) error {
					dl.Status = data.StatusError
					return nil
				})
				return nil, derr
			}
		}
		if _, err := ds.repo.Update(ctx, id, func(dl *data.Download) error {
			dl.GID = ""
			dl.Status = data.StatusCancelled
			return nil
		}); err != nil {
			return nil, err
		}
	}

	// Return the latest snapshot.
	return ds.repo.Get(ctx, id)
}

func isDownloaderNotFound(err error) bool {
	return errors.Is(err, downloader.ErrNotFound)
}

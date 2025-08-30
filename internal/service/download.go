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

type Download interface {
	List(ctx context.Context) (data.Downloads, error)
	Get(ctx context.Context, id int) (*data.Download, error)
	Add(ctx context.Context, d *data.Download) (*data.Download, error)
	UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)
}

var (
	AllowedStatuses = map[data.DownloadStatus]bool{
		data.StatusActive:    true,
		data.StatusPaused:    true,
		data.StatusCancelled: true,
	}
)

type download struct {
	repo repo.DownloadRepo
	dlr  downloader.Downloader
}

func NewDownload(repo repo.DownloadRepo, dlr downloader.Downloader) Download {
	return &download{
		repo: repo,
		dlr:  dlr,
	}
}

func (ds *download) List(ctx context.Context) (data.Downloads, error) {
	return ds.repo.List(ctx)
}

func (ds *download) Get(ctx context.Context, id int) (*data.Download, error) {
	return ds.repo.Get(ctx, id)
}

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
		go func(id int) {
			gid, _ := ds.dlr.Start(context.Background(), saved)
			saved.GID = gid
		}(saved.ID)
	}
	return saved, nil
}

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
	if _, err := ds.repo.UpdateDesiredStatus(ctx, id, status); err != nil {
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
				_ = ds.repo.SetStatus(ctx, id, data.StatusError)
				return nil, derr
			}
			if err := ds.repo.SetGID(ctx, id, gid); err != nil {
				_ = ds.repo.SetStatus(ctx, id, data.StatusError)
				return nil, err
			}
		}
		if err := ds.repo.SetStatus(ctx, id, data.StatusActive); err != nil {
			return nil, err
		}

	case data.StatusPaused:
		// If no GID, nothing to pause (treat as success: desired=Paused, status=Paused).
		if cur.GID != "" {
			if derr := ds.dlr.Pause(ctx, cur); derr != nil {
				_ = ds.repo.SetStatus(ctx, id, data.StatusError)
				return nil, derr
			}
		}
		if err := ds.repo.SetStatus(ctx, id, data.StatusPaused); err != nil {
			return nil, err
		}

	case data.StatusCancelled:
		// Try to cancel if we have a GID; treat "not found" as success for idempotency.
		if cur.GID != "" {
			if derr := ds.dlr.Cancel(ctx, cur); derr != nil && !isDownloaderNotFound(derr) {
				_ = ds.repo.SetStatus(ctx, id, data.StatusError)
				return nil, derr
			}
		}
		if err := ds.repo.ClearGID(ctx, id); err != nil {
			return nil, err
		}
		if err := ds.repo.SetStatus(ctx, id, data.StatusCancelled); err != nil {
			return nil, err
		}
	}

	// Return the latest snapshot.
	return ds.repo.Get(ctx, id)
}

func isDownloaderNotFound(err error) bool {
	return errors.Is(err, downloader.ErrNotFound)
}

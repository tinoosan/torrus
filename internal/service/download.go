package service

import (
	"context"
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
			_ = ds.dlr.Start(context.Background(), id)
		}(saved.ID)
	}
	return saved, nil
}

func (ds *download) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	if !AllowedStatuses[status] {
		return nil, data.ErrBadStatus
	}

	d, err := ds.repo.UpdateDesiredStatus(ctx, id, status)
	if err != nil {
		return nil, err
	}
	var derr error
	switch status {
	case data.StatusActive:
		derr = ds.dlr.Start(context.Background(), id)
	case data.StatusPaused:
		derr = ds.dlr.Pause(context.Background(), id)
	case data.StatusCancelled:
		derr = ds.dlr.Cancel(context.Background(), id)
	}

	if derr != nil {
		_ = ds.repo.SetStatus(ctx, id, data.StatusError)
		return nil, derr
	}

	if err := ds.repo.SetStatus(ctx, id, status); err != nil {
		return nil, err
	}
	d.Status = status
	return d, nil
}

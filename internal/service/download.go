package service

import (
	"context"
	"strings"
	"time"

	"github.com/tinoosan/torrus/internal/data"
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
}

func NewDownload (repo repo.DownloadRepo) Download {
	return &download{
		repo: repo,
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
	if d.DesiredStatus == "" {
		d.DesiredStatus = data.StatusQueued
	}

	if d.Status == "" {
		d.Status = data.StatusQueued
	}
	return ds.repo.Add(ctx, d)
}

func (ds *download) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	if !AllowedStatuses[status] {
		return nil, data.ErrBadStatus
	}
	return ds.repo.UpdateDesiredStatus(ctx, id, status)
}

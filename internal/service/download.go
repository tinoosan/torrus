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
	case "":
		d.DesiredStatus = data.StatusActive
		d.Status = data.StatusActive
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

	go func(s data.DownloadStatus, id int) {
		switch s {
		case data.StatusActive:
			ds.dlr.Start(context.Background(), id)
			ds.repo.SetStatus(context.Background(), id, s)
		case data.StatusPaused:
			ds.dlr.Pause(context.Background(), id)
			ds.repo.SetStatus(context.Background(), id, s)
		case data.StatusCancelled:
			ds.dlr.Cancel(context.Background(), id)
			ds.repo.SetStatus(context.Background(), id, s)
		}
	}(status, id)
	return d, nil
}

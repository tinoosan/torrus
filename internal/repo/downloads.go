package repo

import (
	"context"

	"github.com/tinoosan/torrus/internal/data"
)

type DownloadRepo interface {
	DownloadReader
	DownloadWriter
}

type DownloadReader interface {
	List(ctx context.Context) (data.Downloads, error)
	Get(ctx context.Context, id int) (*data.Download, error)
}

type DownloadWriter interface {
	Add(ctx context.Context, download *data.Download) (*data.Download, error)
	UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)
	SetStatus(ctx context.Context, id int, status data.DownloadStatus) error
}

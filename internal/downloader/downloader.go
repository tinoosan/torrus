package downloader

import (
	"context"
	"errors"

	"github.com/tinoosan/torrus/internal/data"
)

var ErrNotFound = errors.New("downloader not found")

type Downloader interface {
	Start(ctx context.Context, d *data.Download) (string, error)
	Pause(ctx context.Context, d *data.Download) error
	Cancel(ctx context.Context, d *data.Download) error
}

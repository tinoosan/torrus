package downloader

import (
	"context"

	"github.com/tinoosan/torrus/internal/data"
)

type Downloader interface {
	Start(ctx context.Context, d *data.Download) error
	Pause(ctx context.Context, d *data.Download) error
	Cancel(ctx context.Context, d *data.Download) error
}

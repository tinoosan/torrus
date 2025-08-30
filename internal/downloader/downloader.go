package downloader

import (
	"context"
)

type Downloader interface {
	Start(ctx context.Context, id int) error
	Pause(ctx context.Context, id int) error
	Cancel(ctx context.Context, id int) error
}

package downloader

import (
	"context"
	"fmt"

	"github.com/tinoosan/torrus/internal/data"
)

type noopDownloader struct {}

func NewNoopDownloader() Downloader {
	return &noopDownloader{}
}

func (d *noopDownloader) Start(ctx context.Context, dl *data.Download) error {
	fmt.Println("noop: start", dl.ID)
	return nil
}


func (d *noopDownloader) Pause(ctx context.Context, dl *data.Download) error {
	fmt.Println("noop: pause", dl.ID)
	return nil
}


func (d *noopDownloader) Cancel(ctx context.Context, dl *data.Download) error {
	fmt.Println("noop: cancel", dl.ID)
	return nil
}

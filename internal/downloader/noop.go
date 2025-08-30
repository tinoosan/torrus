package downloader

import (
	"context"
	"fmt"
)

type noopDownloader struct {}

func NewNoopDownloader() Downloader {
	return &noopDownloader{}
}

func (d *noopDownloader) Start(ctx context.Context, id int) error {
	fmt.Println("noop: start", id)
	return nil
}


func (d *noopDownloader) Resume(ctx context.Context, id int) error {
	fmt.Println("noop: resume", id)
	return nil
}


func (d *noopDownloader) Pause(ctx context.Context, id int) error {
	fmt.Println("noop: pause", id)
	return nil
}


func (d *noopDownloader) Cancel(ctx context.Context, id int) error {
	fmt.Println("noop: cancel", id)
	return nil
}

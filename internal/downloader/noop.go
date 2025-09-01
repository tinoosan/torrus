package downloader

import (
	"context"
	"fmt"

	"github.com/tinoosan/torrus/internal/data"
)

// noopDownloader implements Downloader but only logs calls.
type noopDownloader struct{}

// NewNoopDownloader returns a Downloader that performs no actions, useful for
// testing and development.
func NewNoopDownloader() Downloader {
	return &noopDownloader{}
}

// Start logs the start request and returns the download ID as a fake GID.
func (d *noopDownloader) Start(ctx context.Context, dl *data.Download) (string, error) {
	fmt.Println("noop: start", dl.ID)
	return dl.ID, nil
}

// Pause logs the pause request and does nothing else.
func (d *noopDownloader) Pause(ctx context.Context, dl *data.Download) error {
	fmt.Println("noop: pause", dl.ID)
	return nil
}

// Resume logs the resume request and does nothing else.
func (d *noopDownloader) Resume(ctx context.Context, dl *data.Download) error {
	fmt.Println("noop: resume", dl.ID)
	return nil
}

// Cancel logs the cancel request and does nothing else.
func (d *noopDownloader) Cancel(ctx context.Context, dl *data.Download) error {
	fmt.Println("noop: cancel", dl.ID)
	return nil
}

// Delete logs the delete request and does nothing else. If deleteFiles is true
// it still only logs and pretends success.
func (d *noopDownloader) Delete(ctx context.Context, dl *data.Download, deleteFiles bool) error {
	if deleteFiles {
		fmt.Println("noop: delete with files", dl.ID)
	} else {
		fmt.Println("noop: delete", dl.ID)
	}
	return nil
}

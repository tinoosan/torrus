package downloader

import "github.com/tinoosan/torrus/internal/data"

type Event struct {
	ID     int
	GID    string
	Status data.DownloadStatus
}

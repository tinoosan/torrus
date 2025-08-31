package repo

import (
	"context"
	"sync"

	"github.com/tinoosan/torrus/internal/data"
)

// InMemoryDownloadRepo stores downloads in memory. It is intended for tests and
// development usage and is not safe for persistence across restarts.
type InMemoryDownloadRepo struct {
	mu        sync.RWMutex
	downloads data.Downloads
	nextID    int
}

// NewInMemoryDownloadRepo returns an initialized in-memory repository.
func NewInMemoryDownloadRepo() *InMemoryDownloadRepo {
	return &InMemoryDownloadRepo{
		downloads: make(data.Downloads, 0),
		nextID:    1,
	}
}

// List returns all stored downloads.
func (r *InMemoryDownloadRepo) List(ctx context.Context) (data.Downloads, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.downloads.Clone(), nil
}

// Get retrieves a download by its ID.
func (r *InMemoryDownloadRepo) Get(ctx context.Context, id int) (*data.Download, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.downloads {
		if d.ID == id {
			return d.Clone(), nil
		}
	}
	return nil, data.ErrNotFound
}

// Add inserts a new download and assigns it a unique ID.
func (r *InMemoryDownloadRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d.ID = r.nextID
	r.nextID++
	r.downloads = append(r.downloads, d)
	return d.Clone(), nil
}

// UpdateDesiredStatus sets the desired status for a download.
func (r *InMemoryDownloadRepo) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dl, err := r.findByID(id)
	if err != nil {
		return nil, err
	}
	dl.DesiredStatus = status
	return dl.Clone(), nil
}

// SetStatus updates the current status of a download.
func (r *InMemoryDownloadRepo) SetStatus(ctx context.Context, id int, status data.DownloadStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dl, err := r.findByID(id)
	if err != nil {
		return err
	}
	dl.Status = status
	return nil
}

// SetGID associates a downloader GID with the download.
func (r *InMemoryDownloadRepo) SetGID(ctx context.Context, id int, gid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dl, err := r.findByID(id)
	if err != nil {
		return err
	}
	dl.GID = gid
	return nil
}

// ClearGID removes any downloader GID from the download.
func (r *InMemoryDownloadRepo) ClearGID(ctx context.Context, id int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	dl, err := r.findByID(id)
	if err != nil {
		return err
	}
	dl.GID = ""
	return nil
}

func (r *InMemoryDownloadRepo) findByID(id int) (*data.Download, error) {
	for _, dl := range r.downloads {
		if dl.ID == id {
			return dl, nil
		}
	}
	return nil, data.ErrNotFound
}

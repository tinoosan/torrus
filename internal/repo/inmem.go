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

// Update applies modifications specified in UpdateFields to the download with
// the given ID. Nil fields in uf leave the corresponding values unchanged. The
// returned download is a deep clone of the stored entity.
func (r *InMemoryDownloadRepo) Update(ctx context.Context, id int, uf UpdateFields) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dl, err := r.findByID(id)
	if err != nil {
		return nil, err
	}

	if uf.DesiredStatus != nil {
		dl.DesiredStatus = *uf.DesiredStatus
	}
	if uf.Status != nil {
		dl.Status = *uf.Status
	}
	if uf.GID != nil {
		dl.GID = *uf.GID
	}

	return dl.Clone(), nil
}

// UpdateDesiredStatus sets the desired status for a download.
func (r *InMemoryDownloadRepo) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	return r.Update(ctx, id, UpdateFields{DesiredStatus: &status})
}

// SetStatus updates the current status of a download.
func (r *InMemoryDownloadRepo) SetStatus(ctx context.Context, id int, status data.DownloadStatus) error {
	_, err := r.Update(ctx, id, UpdateFields{Status: &status})
	return err
}

// SetGID associates a downloader GID with the download.
func (r *InMemoryDownloadRepo) SetGID(ctx context.Context, id int, gid string) error {
	_, err := r.Update(ctx, id, UpdateFields{GID: &gid})
	return err
}

// ClearGID removes any downloader GID from the download.
func (r *InMemoryDownloadRepo) ClearGID(ctx context.Context, id int) error {
	empty := ""
	_, err := r.Update(ctx, id, UpdateFields{GID: &empty})
	return err
}

func (r *InMemoryDownloadRepo) findByID(id int) (*data.Download, error) {
	for _, dl := range r.downloads {
		if dl.ID == id {
			return dl, nil
		}
	}
	return nil, data.ErrNotFound
}

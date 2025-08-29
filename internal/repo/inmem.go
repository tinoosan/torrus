package repo

import (
	"context"
	"sync"
	"time"

	"github.com/tinoosan/torrus/internal/data"
)

type InMemoryDownloadRepo struct {
	mu        sync.RWMutex
	downloads data.Downloads
	nextID    int
}

func NewInMemoryDownloadRepo() *InMemoryDownloadRepo {
	return &InMemoryDownloadRepo{
		downloads: make(data.Downloads, 0),
		nextID: 1,
	}
}

func (r *InMemoryDownloadRepo) List(ctx context.Context) data.Downloads {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(data.Downloads, len(r.downloads))
	copy(out, r.downloads)
	return out
}

func (r *InMemoryDownloadRepo) Get(ctx context.Context, id int) (*data.Download, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.downloads {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, data.ErrNotFound
}

func (r *InMemoryDownloadRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d.ID = r.nextID
	r.nextID++
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	d.DesiredStatus = data.StatusQueued
	d.Status = data.StatusQueued
	r.downloads = append(r.downloads, d)
	return d, nil
}

func (r *InMemoryDownloadRepo) UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !data.AllowedStatuses[status] {
		return nil, data.ErrBadStatus
	}
	dl, err := r.findByID(id)
	if err != nil {
		return nil, err
	}
	dl.DesiredStatus = status
	return dl, nil
}

func (r *InMemoryDownloadRepo) findByID(id int) (*data.Download, error) {
	for _, dl := range r.downloads {
		if dl.ID == id {
			return dl, nil
		}
	}
	return nil, data.ErrNotFound

}

package repo

import (
	"context"
	"sync"

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
		nextID:    1,
	}
}

func (r *InMemoryDownloadRepo) List(ctx context.Context) (data.Downloads, error){
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.downloads.Clone(), nil
}

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

func (r *InMemoryDownloadRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d.ID = r.nextID
	r.nextID++
	r.downloads = append(r.downloads, d)
	return d.Clone(), nil
}

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

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
    // fpIndex maps fingerprint -> index into downloads slice
    fpIndex   map[string]int
}

// NewInMemoryDownloadRepo returns an initialized in-memory repository.
func NewInMemoryDownloadRepo() *InMemoryDownloadRepo {
    return &InMemoryDownloadRepo{
        downloads: make(data.Downloads, 0),
        nextID:    1,
        fpIndex:   make(map[string]int),
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

// GetByFingerprint returns a clone of the download that matches the provided
// fingerprint, or data.ErrNotFound if none exists.
func (r *InMemoryDownloadRepo) GetByFingerprint(ctx context.Context, fp string) (*data.Download, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    idx, ok := r.fpIndex[fp]
    if !ok || idx < 0 || idx >= len(r.downloads) {
        return nil, data.ErrNotFound
    }
    return r.downloads[idx].Clone(), nil
}

// AddWithFingerprint atomically checks if a fingerprint already exists; if so,
// it returns the existing download and created=false. Otherwise it inserts the
// provided download, assigns a new ID, indexes the fingerprint and returns
// created=true.
func (r *InMemoryDownloadRepo) AddWithFingerprint(ctx context.Context, d *data.Download, fp string) (*data.Download, bool, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    if idx, ok := r.fpIndex[fp]; ok {
        if idx >= 0 && idx < len(r.downloads) {
            return r.downloads[idx].Clone(), false, nil
        }
        // fallthrough to reinsert if index out of range (shouldn't happen)
    }

    d.ID = r.nextID
    r.nextID++
    r.downloads = append(r.downloads, d)
    r.fpIndex[fp] = len(r.downloads) - 1
    return d.Clone(), true, nil
}

// Update applies the mutate function to the download with the given ID and
// returns a deep clone of the updated entity. mutate is executed while holding
// the repo lock to ensure atomicity.
func (r *InMemoryDownloadRepo) Update(ctx context.Context, id int, mutate func(*data.Download) error) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dl, err := r.findByID(id)
	if err != nil {
		return nil, err
	}

	if mutate != nil {
		err = mutate(dl)
		if err != nil {
			return nil, err
		}
	}

	return dl.Clone(), nil

}

func (r *InMemoryDownloadRepo) findByID(id int) (*data.Download, error) {
	for _, dl := range r.downloads {
		if dl.ID == id {
			return dl, nil
		}
	}
	return nil, data.ErrNotFound
}

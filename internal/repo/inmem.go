package repo

import (
	"context"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/fp"
)

// InMemoryDownloadRepo stores downloads in memory using a map keyed by ID.
// It is intended for tests and development usage and is not safe for
// persistence across restarts.
type InMemoryDownloadRepo struct {
	mu      sync.RWMutex
	byID    map[string]*data.Download
	fpIndex map[string]string // fingerprint -> id
}

// NewInMemoryDownloadRepo returns an initialized in-memory repository.
func NewInMemoryDownloadRepo() *InMemoryDownloadRepo {
	return &InMemoryDownloadRepo{
		byID:    make(map[string]*data.Download),
		fpIndex: make(map[string]string),
	}
}

// List returns all stored downloads sorted by creation time ascending.
func (r *InMemoryDownloadRepo) List(ctx context.Context) (data.Downloads, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(data.Downloads, 0, len(r.byID))
	for _, d := range r.byID {
		res = append(res, d.Clone())
	}
	sort.Slice(res, func(i, j int) bool { return res[i].CreatedAt.Before(res[j].CreatedAt) })
	return res, nil
}

// Get retrieves a download by its ID.
func (r *InMemoryDownloadRepo) Get(ctx context.Context, id string) (*data.Download, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.byID[id]
	if !ok {
		return nil, data.ErrNotFound
	}
	return d.Clone(), nil
}

// Add inserts a new download and assigns it a unique ID.
func (r *InMemoryDownloadRepo) Add(ctx context.Context, d *data.Download) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d.ID = uuid.NewString()
	r.byID[d.ID] = d
	return d.Clone(), nil
}

// GetByFingerprint returns a clone of the download that matches the provided
// fingerprint, or data.ErrNotFound if none exists.
func (r *InMemoryDownloadRepo) GetByFingerprint(ctx context.Context, fpv string) (*data.Download, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.fpIndex[fpv]
	if !ok {
		return nil, data.ErrNotFound
	}
	dl, ok := r.byID[id]
	if !ok {
		return nil, data.ErrNotFound
	}
	return dl.Clone(), nil
}

// AddWithFingerprint atomically checks if a fingerprint already exists; if so,
// it returns the existing download and created=false. Otherwise it inserts the
// provided download, assigns a new ID, indexes the fingerprint and returns
// created=true.
func (r *InMemoryDownloadRepo) AddWithFingerprint(ctx context.Context, d *data.Download, fpv string) (*data.Download, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id, ok := r.fpIndex[fpv]; ok {
		if dl, ok := r.byID[id]; ok {
			return dl.Clone(), false, nil
		}
	}

	d.ID = uuid.NewString()
	r.byID[d.ID] = d
	r.fpIndex[fpv] = d.ID
	return d.Clone(), true, nil
}

// Update applies the mutate function to the download with the given ID and
// returns a deep clone of the updated entity. mutate is executed while holding
// the repo lock to ensure atomicity.
func (r *InMemoryDownloadRepo) Update(ctx context.Context, id string, mutate func(*data.Download) error) (*data.Download, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dl, ok := r.byID[id]
	if !ok {
		return nil, data.ErrNotFound
	}
	if mutate != nil {
		if err := mutate(dl); err != nil {
			return nil, err
		}
	}
	return dl.Clone(), nil
}

// Delete removes the download with the given ID.
func (r *InMemoryDownloadRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dl, ok := r.byID[id]
	if !ok {
		return data.ErrNotFound
	}
	fpv := fp.Fingerprint(dl.Source, dl.TargetPath)
	delete(r.byID, id)
	delete(r.fpIndex, fpv)
	return nil
}

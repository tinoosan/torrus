package reconciler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
	"github.com/tinoosan/torrus/internal/repo"
)

// Reconciler consumes downloader events and updates repository state.
type Reconciler struct {
	repo   repo.DownloadWriter
	events <-chan downloader.Event
	log    *slog.Logger

	stop chan struct{}
	wg   sync.WaitGroup
}

func New(log *slog.Logger, repo repo.DownloadWriter, events <-chan downloader.Event) *Reconciler {
	if log == nil {
		log = slog.Default()
	}
	return &Reconciler{repo: repo, events: events, log: log}
}

// Run starts the reconciliation loop.
func (r *Reconciler) Run() {
	r.stop = make(chan struct{})
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for {
			select {
			case <-r.stop:
				return
			case e, ok := <-r.events:
				if !ok {
					return
				}
				r.handle(e)
			}
		}
	}()
}

// Stop terminates the reconciliation loop.
func (r *Reconciler) Stop() {
	if r.stop != nil {
		close(r.stop)
		r.wg.Wait()
	}
}

func (r *Reconciler) handle(e downloader.Event) {
	if err := r.repo.SetStatus(context.Background(), e.ID, e.Status); err != nil {
		r.log.Error("set status", "id", e.ID, "status", e.Status, "err", err)
		return
	}
	switch e.Status {
	case data.StatusCancelled, data.StatusComplete, data.StatusError:
		if err := r.repo.ClearGID(context.Background(), e.ID); err != nil {
			r.log.Error("clear gid", "id", e.ID, "err", err)
		}
	}
	r.log.Info("reconciled event", "id", e.ID, "status", e.Status)
}

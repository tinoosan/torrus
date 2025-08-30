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
	var status data.DownloadStatus
	switch e.Type {
	case downloader.EventStart:
		status = data.StatusActive
	case downloader.EventPaused:
		status = data.StatusPaused
	case downloader.EventCancelled:
		status = data.StatusCancelled
	case downloader.EventComplete:
		status = data.StatusComplete
	case downloader.EventFailed:
		status = data.StatusError
	case downloader.EventProgress:
		if e.Progress != nil {
			r.log.Info("progress event", "id", e.ID, "completed", e.Progress.Completed, "total", e.Progress.Total)
		} else {
			r.log.Info("progress event", "id", e.ID)
		}
		return
	default:
		r.log.Warn("unknown event type", "id", e.ID, "type", e.Type)
		return
	}

	if err := r.repo.SetStatus(context.Background(), e.ID, status); err != nil {
		r.log.Error("set status", "id", e.ID, "status", status, "err", err)
		return
	}

	switch e.Type {
	case downloader.EventCancelled, downloader.EventComplete, downloader.EventFailed:
		if err := r.repo.ClearGID(context.Background(), e.ID); err != nil {
			r.log.Error("clear gid", "id", e.ID, "err", err)
		}
	}
	r.log.Info("reconciled event", "id", e.ID, "type", e.Type)
}

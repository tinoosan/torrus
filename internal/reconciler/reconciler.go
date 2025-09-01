package reconciler

import (
    "context"
    "log/slog"
    "sync"
    "strings"

    "github.com/google/uuid"
    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloader"
    "github.com/tinoosan/torrus/internal/metrics"
    "github.com/tinoosan/torrus/internal/repo"
)

// Reconciler consumes downloader events and updates repository state.
type Reconciler struct {
	repo   repo.DownloadRepo
	events <-chan downloader.Event
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc

	stop chan struct{}
	wg   sync.WaitGroup
}

// New creates a Reconciler that processes downloader events and mutates the
// repository accordingly.
func New(log *slog.Logger, repo repo.DownloadRepo, events <-chan downloader.Event) *Reconciler {
	if log == nil {
		log = slog.Default()
	}
	return &Reconciler{repo: repo, events: events, log: log, ctx: context.Background()}
}

// Run starts the reconciliation loop.
func (r *Reconciler) Run() {
    r.stop = make(chan struct{})
    r.ctx, r.cancel = context.WithCancel(r.ctx)
    // Tag this run with a stable operation_id for easier correlation.
    opID := uuid.NewString()
    r.log = r.log.With("operation_id", opID)
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
		if r.cancel != nil {
			r.cancel()
		}
		r.wg.Wait()
	}
}

func (r *Reconciler) handle(e downloader.Event) {
    // Record event type for observability
    metrics.DownloadEvents.WithLabelValues(strings.ToLower(string(e.Type))).Inc()
    var (
        status        data.DownloadStatus
        checkTerminal bool
    )
	switch e.Type {
	case downloader.EventStart:
		dl, err := r.repo.Get(r.ctx, e.ID)
		if err != nil {
			r.log.Error("get", "id", e.ID, "err", err)
			return
		}
		if dl.DesiredStatus != data.StatusActive || dl.Status != data.StatusQueued {
			r.log.Info("ignoring stale start event", "id", e.ID, "status", dl.Status, "desired", dl.DesiredStatus)
			return
		}
		status = data.StatusActive
	case downloader.EventPaused:
		status = data.StatusPaused
	case downloader.EventCancelled:
		status = data.StatusCancelled
		checkTerminal = true
	case downloader.EventComplete:
		status = data.StatusComplete
		checkTerminal = true
	case downloader.EventFailed:
		status = data.StatusError
		checkTerminal = true
	case downloader.EventGIDUpdate:
		if e.NewGID == "" {
			return
		}
		_, err := r.repo.Update(r.ctx, e.ID, func(dl *data.Download) error {
			dl.GID = e.NewGID
			return nil
		})
		if err != nil {
			r.log.Error("update gid", "id", e.ID, "err", err)
		} else {
			r.log.Info("updated gid", "id", e.ID, "gid", e.NewGID)
		}
		return
	case downloader.EventMeta:
		if e.Meta == nil {
			return
		}
		if e.Meta.Name != nil {
			_, err := r.repo.Update(r.ctx, e.ID, func(dl *data.Download) error {
				dl.Name = *e.Meta.Name
				return nil
			})
			if err != nil {
				r.log.Error("update meta", "id", e.ID, "err", err)
			} else {
				r.log.Info("updated meta", "id", e.ID, "name", *e.Meta.Name)
			}
		}
		if e.Meta.Files != nil {
			// Persist files list (read-only field populated by downloader)
			_, err := r.repo.Update(r.ctx, e.ID, func(dl *data.Download) error {
				// Replace the slice to reflect latest snapshot from downloader
				dl.Files = make([]data.DownloadFile, len(*e.Meta.Files))
				copy(dl.Files, *e.Meta.Files)
				return nil
			})
			if err != nil {
				r.log.Error("update files", "id", e.ID, "err", err)
			} else {
				r.log.Info("updated files", "id", e.ID, "count", len(*e.Meta.Files))
			}
		}
		return
	case downloader.EventProgress:
		if e.Progress != nil {
			r.log.Info("progress event", "id", e.ID, "completed", e.Progress.Completed, "total", e.Progress.Total, "speed", e.Progress.Speed)
		} else {
			r.log.Info("progress event", "id", e.ID)
		}
		return
	default:
		r.log.Warn("unknown event type", "id", e.ID, "type", e.Type)
		return
	}

	if checkTerminal && !r.checkTerminal(e) {
		return
	}

	_, err := r.repo.Update(r.ctx, e.ID, func(dl *data.Download) error {
		dl.Status = status
		if checkTerminal {
			dl.GID = ""
		}
		return nil
	})
	if err != nil {
		r.log.Error("update", "id", e.ID, "status", status, "err", err)
		return
	}
	r.log.Info("reconciled event", "id", e.ID, "type", e.Type)
}

func (r *Reconciler) checkTerminal(e downloader.Event) bool {
	dl, err := r.repo.Get(r.ctx, e.ID)
	if err != nil {
		r.log.Error("get", "id", e.ID, "err", err)
		return false
	}
	if dl.GID != "" && dl.GID != e.GID {
		r.log.Info("ignoring stale terminal event", "id", e.ID, "gid", dl.GID, "event_gid", e.GID)
		return false
	}
	return true
}

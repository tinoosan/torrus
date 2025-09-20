package aria2dl

import (
    "os"
    "strconv"
    "sync"

    "log/slog"

    "github.com/tinoosan/torrus/internal/aria2"
    "github.com/tinoosan/torrus/internal/downloader"
)

type fsOps interface {
    Remove(string) error
    RemoveAll(string) error
}

type osFS struct{}

func (osFS) Remove(p string) error    { return os.Remove(p) }
func (osFS) RemoveAll(p string) error { return os.RemoveAll(p) }

// Adapter implements the Downloader interface using an aria2 JSON-RPC client.
// It translates Torrus download operations into aria2 RPC calls.
type Adapter struct {
    cl  *aria2.Client
    rep downloader.Reporter

    mu         sync.RWMutex
    gidToID    map[string]string
    activeGIDs map[string]struct{}
    lastProg   map[string]downloader.Progress
    pollMS     int
    log        *slog.Logger
    fs         fsOps
}

// NewAdapter creates a new Adapter using the provided aria2 client and reporter.
func NewAdapter(cl *aria2.Client, rep downloader.Reporter) *Adapter {
    poll := 1000
    if v := os.Getenv("ARIA2_POLL_MS"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            poll = n
        }
    }
    return &Adapter{cl: cl, rep: rep, gidToID: make(map[string]string), activeGIDs: make(map[string]struct{}), lastProg: make(map[string]downloader.Progress), pollMS: poll, log: slog.Default(), fs: osFS{}}
}

var _ downloader.Downloader = (*Adapter)(nil)
var _ downloader.EventSource = (*Adapter)(nil)
var _ downloader.FileLister = (*Adapter)(nil)

// SetLogger allows wiring a shared application logger into the adapter.
func (a *Adapter) SetLogger(l *slog.Logger) {
    if l != nil {
        a.log = l
    }
}


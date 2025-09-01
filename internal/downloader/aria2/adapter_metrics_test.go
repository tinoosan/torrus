package aria2dl

import (
    "context"
    "testing"

    "github.com/prometheus/client_golang/prometheus/testutil"
    "github.com/tinoosan/torrus/internal/aria2"
    "github.com/tinoosan/torrus/internal/metrics"
)

// Ensure that handling a stop notification updates the ActiveDownloads gauge.
func TestActiveDownloadsGaugeOnStop(t *testing.T) {
    a := NewAdapter((*aria2.Client)(nil), nil)

    // Seed one active GID and gid->id map entry
    a.mu.Lock()
    a.activeGIDs["g1"] = struct{}{}
    a.gidToID["g1"] = "id1"
    a.mu.Unlock()

    // Pretend previous gauge value was 1 so we can verify change to 0
    metrics.ActiveDownloads.Set(1)

    n := aria2.Notification{Method: "aria2.onDownloadStop", Params: []aria2.NotificationEvent{{GID: "g1"}}}
    a.handleNotification(context.Background(), n)

    if got := testutil.ToFloat64(metrics.ActiveDownloads); got != 0 {
        t.Fatalf("active_downloads gauge = %v, want 0", got)
    }
}


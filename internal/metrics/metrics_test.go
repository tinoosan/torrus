package metrics

import (
    "strings"
    "testing"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCountersAndGauge(t *testing.T) {
    reg := prometheus.NewRegistry()
    reg.MustRegister(DownloadEvents, Aria2RPCErrors, Aria2RPCLatency, ActiveDownloads)

    DownloadEvents.WithLabelValues("start").Inc()
    Aria2RPCErrors.WithLabelValues("aria2.getVersion").Add(2)
    ActiveDownloads.Set(3)

    // Histogram: observe one sample to ensure collector is live
    Aria2RPCLatency.WithLabelValues("aria2.getVersion").Observe(0.05)

    // Verify DownloadEvents
    expectedEvents := `# HELP torrus_download_events_total Count of download events processed by the reconciler.
# TYPE torrus_download_events_total counter
torrus_download_events_total{type="start"} 1
`
    if err := testutil.CollectAndCompare(DownloadEvents, strings.NewReader(expectedEvents)); err != nil {
        t.Fatalf("unexpected events metric: %v", err)
    }

    // Verify Aria2RPCErrors
    expectedErrors := `# HELP torrus_aria2_rpc_errors_total Errors from aria2 JSON-RPC calls.
# TYPE torrus_aria2_rpc_errors_total counter
torrus_aria2_rpc_errors_total{method="aria2.getVersion"} 2
`
    if err := testutil.CollectAndCompare(Aria2RPCErrors, strings.NewReader(expectedErrors)); err != nil {
        t.Fatalf("unexpected aria2 errors metric: %v", err)
    }

    // Verify ActiveDownloads
    expectedGauge := `# HELP torrus_active_downloads Number of active downloads tracked by the adapter.
# TYPE torrus_active_downloads gauge
torrus_active_downloads 3
`
    if err := testutil.CollectAndCompare(ActiveDownloads, strings.NewReader(expectedGauge)); err != nil {
        t.Fatalf("unexpected active downloads gauge: %v", err)
    }
}

func TestAria2LatencyHistogram(t *testing.T) {
    // Use a fresh histogram to avoid cross-test contamination
    Aria2RPCLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "torrus",
            Name:      "aria2_rpc_latency_seconds",
            Help:      "Latency of aria2 JSON-RPC calls.",
        },
        []string{"method"},
    )

    // Observe two samples and verify default bucket accounting
    Aria2RPCLatency.WithLabelValues("aria2.tellStatus").Observe(0.03)
    Aria2RPCLatency.WithLabelValues("aria2.tellStatus").Observe(0.6)

    expected := `# HELP torrus_aria2_rpc_latency_seconds Latency of aria2 JSON-RPC calls.
# TYPE torrus_aria2_rpc_latency_seconds histogram
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.005"} 0
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.01"} 0
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.025"} 0
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.05"} 1
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.1"} 1
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.25"} 1
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="0.5"} 1
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="1"} 2
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="2.5"} 2
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="5"} 2
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="10"} 2
torrus_aria2_rpc_latency_seconds_bucket{method="aria2.tellStatus",le="+Inf"} 2
torrus_aria2_rpc_latency_seconds_sum{method="aria2.tellStatus"} 0.63
torrus_aria2_rpc_latency_seconds_count{method="aria2.tellStatus"} 2
`
    if err := testutil.CollectAndCompare(Aria2RPCLatency, strings.NewReader(expected)); err != nil {
        t.Fatalf("unexpected histogram: %v", err)
    }
}

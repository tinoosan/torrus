package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    DownloadEvents = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "torrus",
            Name:      "download_events_total",
            Help:      "Count of download events processed by the reconciler.",
        },
        []string{"type"},
    )

    Aria2RPCErrors = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "torrus",
            Name:      "aria2_rpc_errors_total",
            Help:      "Errors from aria2 JSON-RPC calls.",
        },
        []string{"method"},
    )

    Aria2RPCLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "torrus",
            Name:      "aria2_rpc_latency_seconds",
            Help:      "Latency of aria2 JSON-RPC calls.",
        },
        []string{"method"},
    )

    ActiveDownloads = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Namespace: "torrus",
            Name:      "active_downloads",
            Help:      "Number of active downloads tracked by the adapter.",
        },
    )
)

// Register registers the Torrus metrics into the default registry.
func Register() {
    prometheus.MustRegister(DownloadEvents, Aria2RPCErrors, Aria2RPCLatency, ActiveDownloads)
}


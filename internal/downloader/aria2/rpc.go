package aria2dl

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/tinoosan/torrus/internal/metrics"
)

// --- JSON-RPC wire types ---

type rpcReq struct {
    Jsonrpc string        `json:"jsonrpc"`
    Method  string        `json:"method"`
    ID      string        `json:"id"`
    Params  []interface{} `json:"params,omitempty"`
}

type rpcResp struct {
    Jsonrpc string          `json:"jsonrpc"`
    ID      string          `json:"id"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func (a *Adapter) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
    timer := prometheus.NewTimer(metrics.Aria2RPCLatency.WithLabelValues(method))
    defer timer.ObserveDuration()
    body, _ := json.Marshal(rpcReq{Jsonrpc: "2.0", Method: method, ID: "torrus", Params: params})

    req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cl.BaseURL().String(), bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := a.cl.HTTP().Do(req)
    if err != nil {
        metrics.Aria2RPCErrors.WithLabelValues(method).Inc()
        return nil, err
    }
    defer func() { _ = resp.Body.Close() }()

    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        b, _ := io.ReadAll(resp.Body)
        metrics.Aria2RPCErrors.WithLabelValues(method).Inc()
        return nil, fmt.Errorf("aria2 http %d: %s", resp.StatusCode, string(b))
    }
    b, _ := io.ReadAll(resp.Body)

    var rr rpcResp
    if err := json.Unmarshal(b, &rr); err != nil {
        metrics.Aria2RPCErrors.WithLabelValues(method).Inc()
        return nil, fmt.Errorf("aria2 rpc decode: %w (%s)", err, string(b))
    }
    if rr.Error != nil {
        metrics.Aria2RPCErrors.WithLabelValues(method).Inc()
        return nil, fmt.Errorf("aria2 rpc error %d: %s", rr.Error.Code, rr.Error.Message)
    }
    return rr.Result, nil
}

// helper: token parameter if secret set (aria2 expects "token:<secret>" as first param)
func (a *Adapter) tokenParam() []interface{} {
    if s := a.cl.Secret(); s != "" {
        return []interface{}{"token:" + s}
    }
    return nil
}

// isAria2ConflictError attempts to detect a file-collision error from aria2.
func isAria2ConflictError(err error) bool {
    if err == nil {
        return false
    }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "file already exists") || strings.Contains(msg, "file exists")
}

// isAria2GIDNotFoundError detects when aria2 reports a missing GID.
func isAria2GIDNotFoundError(err error) bool {
    if err == nil {
        return false
    }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "gid not found")
}


package aria2dl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/tinoosan/torrus/internal/aria2" // your Client
	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader" // the Downloader interface
)

// Adapter implements the Downloader interface using an aria2 JSON-RPC client.
// It translates Torrus download operations into aria2 RPC calls.
type Adapter struct {
	cl  *aria2.Client
	rep downloader.Reporter
}

// NewAdapter creates a new Adapter using the provided aria2 client and reporter.
func NewAdapter(cl *aria2.Client, rep downloader.Reporter) *Adapter {
	return &Adapter{cl: cl, rep: rep}
}

var _ downloader.Downloader = (*Adapter)(nil)

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
	body, _ := json.Marshal(rpcReq{
		Jsonrpc: "2.0",
		Method:  method,
		ID:      "torrus",
		Params:  params,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cl.BaseURL().String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.cl.HTTP().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aria2 http %d: %s", resp.StatusCode, string(b))
	}

	b, _ := io.ReadAll(resp.Body)

	var rr rpcResp
	if err := json.Unmarshal(b, &rr); err != nil {
		return nil, fmt.Errorf("aria2 rpc decode: %w (%s)", err, string(b))
	}
	if rr.Error != nil {
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

// Start: aria2.addUri([token?, [uris], options])
func (a *Adapter) Start(ctx context.Context, dl *data.Download) (string, error) {
	params := make([]interface{}, 0, 3)
	if tok := a.tokenParam(); tok != nil {
		params = append(params, tok...)
	}
	params = append(params, []string{dl.Source})
	opts := map[string]string{}
	if dl.TargetPath != "" {
		opts["dir"] = dl.TargetPath
	}
	params = append(params, opts)

	res, err := a.call(ctx, "aria2.addUri", params)
	if err != nil {
		return "", err
	}
	// result is GID string
	var gid string
	if err := json.Unmarshal(res, &gid); err != nil {
		return "", fmt.Errorf("parse addUri result: %w", err)
	}
	if a.rep != nil {
		a.rep.Report(downloader.Event{ID: dl.ID, GID: gid, Type: downloader.EventStart})
	}
	return gid, nil
}

// Pause: aria2.pause([token?, gid])
func (a *Adapter) Pause(ctx context.Context, dl *data.Download) error {
	params := append(a.tokenParam(), dl.GID)
	_, err := a.call(ctx, "aria2.pause", params)
	if err == nil && a.rep != nil {
		a.rep.Report(downloader.Event{ID: dl.ID, GID: dl.GID, Type: downloader.EventPaused})
	}
	return err
}

// Cancel: aria2.remove([token?, gid])
func (a *Adapter) Cancel(ctx context.Context, dl *data.Download) error {
	params := append(a.tokenParam(), dl.GID)
	_, err := a.call(ctx, "aria2.remove", params)
	if err == nil && a.rep != nil {
		a.rep.Report(downloader.Event{ID: dl.ID, GID: dl.GID, Type: downloader.EventCancelled})
	}
	return err
}

// EmitComplete can be used by callers to signal that a download finished
// successfully. Typically this would be triggered by an aria2 notification.
func (a *Adapter) EmitComplete(id int, gid string) {
	if a.rep != nil {
		a.rep.Report(downloader.Event{ID: id, GID: gid, Type: downloader.EventComplete})
	}
}

// EmitFailed signals that a download has failed.
func (a *Adapter) EmitFailed(id int, gid string) {
	if a.rep != nil {
		a.rep.Report(downloader.Event{ID: id, GID: gid, Type: downloader.EventFailed})
	}
}

// EmitProgress publishes a progress update for the given download. Callers are
// responsible for providing whatever metrics they have available.
func (a *Adapter) EmitProgress(id int, gid string, p downloader.Progress) {
	if a.rep != nil {
		a.rep.Report(downloader.Event{ID: id, GID: gid, Type: downloader.EventProgress, Progress: &p})
	}
}

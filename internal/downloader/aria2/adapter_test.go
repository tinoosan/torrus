package aria2dl

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/tinoosan/torrus/internal/aria2"
	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestAdapter(t *testing.T, secret string, rt http.RoundTripper) *Adapter {
	t.Helper()
	t.Setenv("ARIA2_RPC_URL", "http://example.com/jsonrpc")
	t.Setenv("ARIA2_SECRET", secret)
	c, err := aria2.NewClientFromEnv()
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	c.HTTP().Transport = rt
	events := make(chan downloader.Event, 1)
	rep := downloader.NewChanReporter(events)
	return NewAdapter(c, rep)
}

func TestAdapterStart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dl := &data.Download{Source: "http://foo/bar", TargetPath: "/tmp"}
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(r.Body)
			var req rpcReq
			err := json.Unmarshal(b, &req)
			if err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Method != "aria2.addUri" {
				t.Fatalf("method = %s", req.Method)
			}
			if req.ID != "torrus" {
				t.Fatalf("id = %s", req.ID)
			}
			if len(req.Params) != 3 {
				t.Fatalf("params len = %d", len(req.Params))
			}
			if tok, _ := req.Params[0].(string); tok != "token:secret" {
				t.Fatalf("token param = %v", req.Params[0])
			}
			if uris, ok := req.Params[1].([]interface{}); !ok || len(uris) != 1 || uris[0] != dl.Source {
				t.Fatalf("uris param = %#v", req.Params[1])
			}
			if opts, ok := req.Params[2].(map[string]interface{}); !ok || opts["dir"] != dl.TargetPath {
				t.Fatalf("opts = %#v", req.Params[2])
			}
			resp := rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"gid123"`)}
			rb, _ := json.Marshal(resp)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		})
		a := newTestAdapter(t, "secret", rt)
		gid, err := a.Start(context.Background(), dl)
		if err != nil {
			t.Fatalf("Start error: %v", err)
		}
		if gid != "gid123" {
			t.Fatalf("gid = %s", gid)
		}
	})

	t.Run("rpc error", func(t *testing.T) {
		dl := &data.Download{Source: "http://foo/bar"}
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(r.Body)
			var req rpcReq
			err := json.Unmarshal(b, &req)
			if err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.Method != "aria2.addUri" {
				t.Fatalf("method = %s", req.Method)
			}
			if req.ID != "torrus" {
				t.Fatalf("id = %s", req.ID)
			}
			if len(req.Params) != 2 {
				t.Fatalf("params len = %d", len(req.Params))
			}
			if _, ok := req.Params[0].([]interface{}); !ok {
				t.Fatalf("expected uris slice, got %#v", req.Params[0])
			}
			resp := rpcResp{Jsonrpc: "2.0", ID: "torrus", Error: &rpcError{Code: 1, Message: "boom"}}
			rb, _ := json.Marshal(resp)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		})
		a := newTestAdapter(t, "", rt)
		gid, err := a.Start(context.Background(), dl)
		if err == nil {
			t.Fatalf("expected error")
		}
		if gid != "" {
			t.Fatalf("gid = %s", gid)
		}
	})
}

func TestAdapterPauseCancel(t *testing.T) {
	methods := []struct {
		name      string
		rpcMethod string
		call      func(context.Context, *Adapter, *data.Download) error
	}{
		{"Pause", "aria2.pause", func(ctx context.Context, a *Adapter, d *data.Download) error { return a.Pause(ctx, d) }},
		{"Cancel", "aria2.remove", func(ctx context.Context, a *Adapter, d *data.Download) error { return a.Cancel(ctx, d) }},
	}

	for _, m := range methods {
		t.Run(m.name+" success", func(t *testing.T) {
			dl := &data.Download{GID: "gid-1"}
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				var req rpcReq
				err := json.Unmarshal(b, &req)
				if err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if req.Method != m.rpcMethod {
					t.Fatalf("method = %s", req.Method)
				}
				if req.ID != "torrus" {
					t.Fatalf("id = %s", req.ID)
				}
				if len(req.Params) != 2 {
					t.Fatalf("params len = %d", len(req.Params))
				}
				if tok, _ := req.Params[0].(string); tok != "token:secret" {
					t.Fatalf("token param = %v", req.Params[0])
				}
				if gid, _ := req.Params[1].(string); gid != dl.GID {
					t.Fatalf("gid param = %v", req.Params[1])
				}
				resp := rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)}
				rb, _ := json.Marshal(resp)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
			})
			a := newTestAdapter(t, "secret", rt)
			err := m.call(context.Background(), a, dl)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})

		t.Run(m.name+" error", func(t *testing.T) {
			dl := &data.Download{GID: "gid-1"}
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				resp := rpcResp{Jsonrpc: "2.0", ID: "torrus", Error: &rpcError{Code: 2, Message: "fail"}}
				rb, _ := json.Marshal(resp)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
			})
			a := newTestAdapter(t, "", rt)
			err := m.call(context.Background(), a, dl)
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestAdapterHandleNotification(t *testing.T) {
	events := make(chan downloader.Event, 2)
	rep := downloader.NewChanReporter(events)
	a := &Adapter{rep: rep, gidToID: map[string]int{"g1": 1, "g2": 2}}

	// Complete event
	a.handleNotification(aria2.Notification{Method: "aria2.onDownloadComplete", Params: []aria2.NotificationEvent{{GID: "g1"}}})
	ev := <-events
	if ev.Type != downloader.EventComplete || ev.ID != 1 || ev.GID != "g1" {
		t.Fatalf("unexpected event %#v", ev)
	}
	if _, ok := a.gidToID["g1"]; ok {
		t.Fatalf("gid not removed after complete")
	}

	// Error event
	a.handleNotification(aria2.Notification{Method: "aria2.onDownloadError", Params: []aria2.NotificationEvent{{GID: "g2"}}})
	ev = <-events
	if ev.Type != downloader.EventFailed || ev.ID != 2 || ev.GID != "g2" {
		t.Fatalf("unexpected event %#v", ev)
	}
}

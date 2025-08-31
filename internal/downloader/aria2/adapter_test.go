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

func newTestAdapterWithEvents(t *testing.T, secret string, rt http.RoundTripper) (*Adapter, chan downloader.Event) {
	t.Helper()
	t.Setenv("ARIA2_RPC_URL", "http://example.com/jsonrpc")
	t.Setenv("ARIA2_SECRET", secret)
	c, err := aria2.NewClientFromEnv()
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	c.HTTP().Transport = rt
	events := make(chan downloader.Event, 4)
	rep := downloader.NewChanReporter(events)
	return NewAdapter(c, rep), events
}

func TestAdapterStart(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dl := &data.Download{ID: 1, Source: "http://example.com/files/movie.mkv", TargetPath: "/tmp"}
		call := 0
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			call++
			b, _ := io.ReadAll(r.Body)
			var req rpcReq
			if err := json.Unmarshal(b, &req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			switch call {
			case 1:
				if req.Method != "aria2.addUri" {
					t.Fatalf("method = %s", req.Method)
				}
				resp := rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"gid123"`)}
				rb, _ := json.Marshal(resp)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
			case 2:
				if req.Method != "aria2.tellStatus" {
					t.Fatalf("expected tellStatus, got %s", req.Method)
				}
				// Return files path to extract name
				result := map[string]any{
					"files": []map[string]any{{"path": "/downloads/movie.mkv"}},
				}
				rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
			case 3:
				if req.Method != "aria2.getFiles" {
					t.Fatalf("expected getFiles, got %s", req.Method)
				}
				// Return one file entry
				result := []map[string]any{{"path": "/downloads/movie.mkv", "length": "1000", "completedLength": "100"}}
				rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
			default:
				t.Fatalf("unexpected extra call %d", call)
				return nil, nil
			}
		})
		a, events := newTestAdapterWithEvents(t, "secret", rt)
		gid, err := a.Start(context.Background(), dl)
		if err != nil {
			t.Fatalf("Start error: %v", err)
		}
		if gid != "gid123" {
			t.Fatalf("gid = %s", gid)
		}
		// Expect Start then Meta
		ev1 := <-events
		if ev1.Type != downloader.EventStart {
			t.Fatalf("first event = %v", ev1.Type)
		}
		ev2 := <-events
		if ev2.Type != downloader.EventMeta || ev2.Meta == nil || ev2.Meta.Name == nil || *ev2.Meta.Name != "movie.mkv" {
			t.Fatalf("unexpected meta event: %#v", ev2)
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

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestAdapterResumeEmitsMeta(t *testing.T) {
	dl := &data.Download{ID: 1, Source: "magnet:?xt=urn:btih:abc&dn=Cool.Name.2024", GID: "gid-9"}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch call {
		case 1:
			if req.Method != "aria2.unpause" {
				t.Fatalf("method = %s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("expected tellStatus, got %s", req.Method)
			}
			// No metadata; adapter should fallback to magnet dn
			result := map[string]any{}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("expected getFiles, got %s", req.Method)
			}
			// No files known yet
			result := []map[string]any{}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected extra call %d", call)
			return nil, nil
		}
	})
	a, events := newTestAdapterWithEvents(t, "secret", rt)
	if err := a.Resume(context.Background(), dl); err != nil {
		t.Fatalf("resume: %v", err)
	}
	// Expect Meta with fallback name
	ev := <-events
	if ev.Type == downloader.EventStart {
		ev = <-events
	}
	if ev.Type != downloader.EventMeta || ev.Meta == nil || ev.Meta.Name == nil || *ev.Meta.Name != "Cool.Name.2024" {
		t.Fatalf("unexpected event: %#v", ev)
	}
}

func TestAdapterEmitsFilesMeta(t *testing.T) {
	dl := &data.Download{ID: 42, Source: "http://example.com/pack", TargetPath: "/tmp"}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch call {
		case 1:
			if req.Method != "aria2.addUri" {
				t.Fatalf("call1 method=%s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"gidxyz"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("call2 method=%s", req.Method)
			}
			// fetchName: return files and bittorrent name
			result := map[string]any{
				"bittorrent": map[string]any{"info": map[string]any{"name": "Show.S01"}},
				"files": []map[string]any{
					{"path": "/downloads/Show.S01/ep1.mkv", "length": "1000", "completedLength": "500"},
					{"path": "/downloads/Show.S01/ep2.mkv", "length": "2000", "completedLength": "0"},
				},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("call3 method=%s", req.Method)
			}
			// getFiles: return files list
			result := []map[string]any{
				{"path": "/downloads/Show.S01/ep1.mkv", "length": "1000", "completedLength": "500"},
				{"path": "/downloads/Show.S01/ep2.mkv", "length": "2000", "completedLength": "0"},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected extra call %d", call)
			return nil, nil
		}
	})
	a, events := newTestAdapterWithEvents(t, "secret", rt)
	gid, err := a.Start(context.Background(), dl)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if gid != "gidxyz" {
		t.Fatalf("gid: %s", gid)
	}
	// Expect Start then Meta with Files
	<-events // Start
	ev := <-events
	if ev.Type != downloader.EventMeta || ev.Meta == nil || ev.Meta.Files == nil {
		t.Fatalf("expected meta with files, got %#v", ev)
	}
	files := *ev.Meta.Files
	if len(files) != 2 {
		t.Fatalf("files len=%d", len(files))
	}
	if files[0].Path != "ep1.mkv" || files[0].Length != 1000 || files[0].Completed != 500 {
		t.Fatalf("file0 mismatch: %#v", files[0])
	}
	if files[1].Path != "ep2.mkv" || files[1].Length != 2000 || files[1].Completed != 0 {
		t.Fatalf("file1 mismatch: %#v", files[1])
	}
}

func TestAdapterFiltersDotFiles(t *testing.T) {
	dl := &data.Download{ID: 7, Source: "http://example.com/onefile", TargetPath: "/tmp"}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch call {
		case 1:
			if req.Method != "aria2.addUri" {
				t.Fatalf("call1 method=%s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"giddot"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("call2 method=%s", req.Method)
			}
			// Return a placeholder path "." and a real file; placeholder should be filtered
			result := map[string]any{
				"files": []map[string]any{
					{"path": "/downloads/.", "length": "0", "completedLength": "0"},
					{"path": "/downloads/real.mkv", "length": "1234", "completedLength": "1000"},
				},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("call3 method=%s", req.Method)
			}
			// getFiles during Start: repeat the same list
			result := []map[string]any{
				{"path": "/downloads/.", "length": "0", "completedLength": "0"},
				{"path": "/downloads/real.mkv", "length": "1234", "completedLength": "1000"},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected extra call %d", call)
			return nil, nil
		}
	})
	a, events := newTestAdapterWithEvents(t, "secret", rt)
	gid, err := a.Start(context.Background(), dl)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if gid != "giddot" {
		t.Fatalf("gid: %s", gid)
	}
	<-events // Start
	ev := <-events
	if ev.Type != downloader.EventMeta || ev.Meta == nil || ev.Meta.Files == nil {
		t.Fatalf("expected meta with files, got %#v", ev)
	}
	files := *ev.Meta.Files
	if len(files) != 1 {
		t.Fatalf("expected 1 file after filtering, got %d", len(files))
	}
	if files[0].Path != "real.mkv" || files[0].Length != 1234 || files[0].Completed != 1000 {
		t.Fatalf("unexpected file: %#v", files[0])
	}
}

func TestAdapterStartMagnetFollowedBySwap(t *testing.T) {
	dl := &data.Download{ID: 11, Source: "magnet:?xt=urn:btih:abc&dn=My.Torrent", TargetPath: "/tmp"}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch call {
		case 1:
			if req.Method != "aria2.addUri" {
				t.Fatalf("call1 method=%s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"meta123"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("call2 method=%s", req.Method)
			}
			// tellStatus shows followedBy real gid and bittorrent name
			result := map[string]any{
				"followedBy": []string{"real456"},
				"bittorrent": map[string]any{"info": map[string]any{"name": "BT.Name"}},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("call3 method=%s", req.Method)
			}
			// getFiles for real gid
			result := []map[string]any{{"path": "/downloads/BT.Name/file1", "length": "10", "completedLength": "2"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected extra call %d", call)
			return nil, nil
		}
	})
	a, events := newTestAdapterWithEvents(t, "secret", rt)
	gid, err := a.Start(context.Background(), dl)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if gid != "real456" {
		t.Fatalf("expected real gid, got %s", gid)
	}
	// Start then Meta
	<-events
	ev := <-events
	if ev.Type != downloader.EventMeta || ev.Meta == nil || ev.Meta.Name == nil || *ev.Meta.Name != "BT.Name" {
		t.Fatalf("unexpected meta: %#v", ev)
	}
}

func TestAdapterResumeFollowedBySwap(t *testing.T) {
	dl := &data.Download{ID: 21, Source: "magnet:?xt=urn:btih:abc&dn=Z.Name", GID: "metaG"}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch call {
		case 1:
			if req.Method != "aria2.unpause" {
				t.Fatalf("call1 method=%s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("call2 method=%s", req.Method)
			}
			result := map[string]any{
				"followedBy": []string{"realG"},
				"files":      []map[string]any{{"path": "/tmp/x"}},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("call3 method=%s", req.Method)
			}
			result := []map[string]any{{"path": "/downloads/real/file.mkv", "length": "5", "completedLength": "1"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected call %d", call)
			return nil, nil
		}
	})
	a, events := newTestAdapterWithEvents(t, "secret", rt)
	if err := a.Resume(context.Background(), dl); err != nil {
		t.Fatalf("resume: %v", err)
	}
	// Expect GIDUpdate then Meta
	ev := <-events
	if ev.Type != downloader.EventGIDUpdate || ev.NewGID != "realG" {
		t.Fatalf("expected gid update, got %#v", ev)
	}
	ev = <-events
	if ev.Type != downloader.EventMeta || ev.Meta == nil || ev.Meta.Files == nil {
		t.Fatalf("expected meta, got %#v", ev)
	}
}

func TestAdapterPauseCancel(t *testing.T) {
	methods := []struct {
		name      string
		rpcMethod string
		call      func(context.Context, *Adapter, *data.Download) error
	}{
		{"Pause", "aria2.pause", func(ctx context.Context, a *Adapter, d *data.Download) error { return a.Pause(ctx, d) }},
		{"Resume", "aria2.unpause", func(ctx context.Context, a *Adapter, d *data.Download) error { return a.Resume(ctx, d) }},
		{"Cancel", "aria2.remove", func(ctx context.Context, a *Adapter, d *data.Download) error { return a.Cancel(ctx, d) }},
	}

	for _, m := range methods {
		t.Run(m.name+" success", func(t *testing.T) {
			dl := &data.Download{GID: "gid-1"}
			first := true
			rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				b, _ := io.ReadAll(r.Body)
				var req rpcReq
				err := json.Unmarshal(b, &req)
				if err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if first {
					first = false
					if req.Method != m.rpcMethod {
						t.Fatalf("method = %s", req.Method)
					}
				} else {
					// For Resume, subsequent tellStatus and getFiles are expected; others should not hit here
					if m.name != "Resume" || (req.Method != "aria2.tellStatus" && req.Method != "aria2.getFiles") {
						t.Fatalf("unexpected extra call: %s", req.Method)
					}
				}
				if req.ID != "torrus" {
					t.Fatalf("id = %s", req.ID)
				}
				// Return success for first call; for tellStatus provide empty result
				if req.Method == m.rpcMethod {
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
				}
				// tellStatus/getFiles response (empty)
				var result json.RawMessage
				if req.Method == "aria2.tellStatus" {
					result = json.RawMessage(`{}`)
				} else {
					result = must(json.Marshal([]any{}))
				}
				rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: result})
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
	a := &Adapter{rep: rep, gidToID: map[string]int{"g1": 1, "g2": 2}, activeGIDs: map[string]struct{}{}, lastProg: map[string]downloader.Progress{}}

	// Complete event
	a.handleNotification(context.Background(), aria2.Notification{Method: "aria2.onDownloadComplete", Params: []aria2.NotificationEvent{{GID: "g1"}}})
	ev := <-events
	if ev.Type != downloader.EventComplete || ev.ID != 1 || ev.GID != "g1" {
		t.Fatalf("unexpected event %#v", ev)
	}
	if _, ok := a.gidToID["g1"]; ok {
		t.Fatalf("gid not removed after complete")
	}

	// Error event
	a.handleNotification(context.Background(), aria2.Notification{Method: "aria2.onDownloadError", Params: []aria2.NotificationEvent{{GID: "g2"}}})
	ev = <-events
	if ev.Type != downloader.EventFailed || ev.ID != 2 || ev.GID != "g2" {
		t.Fatalf("unexpected event %#v", ev)
	}
}

func TestAdapterMetadataCompleteTriggersFollowedBySwap(t *testing.T) {
	// Start with a magnet where immediate followedBy is empty; later a completion
	// notification for the metadata gid should cause a swap to the real gid.
	dl := &data.Download{ID: 33, Source: "magnet:?xt=urn:btih:xyz&dn=Title", TargetPath: "/tmp"}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch call {
		case 1:
			if req.Method != "aria2.addUri" {
				t.Fatalf("call1 method=%s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"metaG"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("call2 method=%s", req.Method)
			}
			// No followedBy yet
			result := map[string]any{}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("call3 method=%s", req.Method)
			}
			// No files yet
			result := []map[string]any{}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 4:
			// handleNotification will call tellStatus on meta gid to look for followedBy
			if req.Method != "aria2.tellStatus" {
				t.Fatalf("call4 method=%s", req.Method)
			}
			result := map[string]any{
				"followedBy": []string{"realG"},
				"bittorrent": map[string]any{"info": map[string]any{"name": "Real.Title"}},
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 5:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("call5 method=%s", req.Method)
			}
			result := []map[string]any{{"path": "/downloads/Real.Title/file1", "length": "10", "completedLength": "2"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected call %d", call)
			return nil, nil
		}
	})
	a, events := newTestAdapterWithEvents(t, "secret", rt)
	gid, err := a.Start(context.Background(), dl)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if gid != "metaG" {
		t.Fatalf("expected initial gid metaG, got %s", gid)
	}
	// Drain initial events (Start + optional Meta)
	<-events
	select {
	case <-events:
	default:
	}
	// Now deliver metadata completion notification; adapter should swap GID
	a.handleNotification(context.Background(), aria2.Notification{Method: "aria2.onDownloadComplete", Params: []aria2.NotificationEvent{{GID: "metaG"}}})
	// Expect GIDUpdate then Meta for real gid
	ev := <-events
	if ev.Type != downloader.EventGIDUpdate || ev.NewGID != "realG" {
		t.Fatalf("expected gid update to realG, got %#v", ev)
	}
	ev = <-events
	if ev.Type != downloader.EventMeta || ev.Meta == nil || ev.Meta.Name == nil || *ev.Meta.Name != "Real.Title" {
		t.Fatalf("expected meta with name Real.Title, got %#v", ev)
	}
}

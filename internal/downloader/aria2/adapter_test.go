package aria2dl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		dl := &data.Download{ID: "1", Source: "http://example.com/files/movie.mkv", TargetPath: "/tmp"}
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

// newAdapterNoRPC creates an adapter with default client and no custom transport.
// Useful for tests that only exercise filesystem behavior (no RPC).
func newAdapterNoRPC(t *testing.T) *Adapter {
    t.Helper()
    c, err := aria2.NewClientFromEnv()
    if err != nil { t.Fatalf("client: %v", err) }
    return NewAdapter(c, nil)
}

func TestAdapterResumeEmitsMeta(t *testing.T) {
	dl := &data.Download{ID: "1", Source: "magnet:?xt=urn:btih:abc&dn=Cool.Name.2024", GID: "gid-9"}
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

func TestAdapterPauseAndCancel(t *testing.T) {
	dl := &data.Download{ID: "1", GID: "gid1"}
	pauseRT := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		if req.Method != "aria2.pause" {
			t.Fatalf("pause method=%s", req.Method)
		}
		rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
	})
	a, events := newTestAdapterWithEvents(t, "", pauseRT)
	if err := a.Pause(context.Background(), dl); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	ev := <-events
	if ev.Type != downloader.EventPaused || ev.ID != dl.ID {
		t.Fatalf("pause event: %#v", ev)
	}

	cancelRT := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		if req.Method != "aria2.remove" {
			t.Fatalf("cancel method=%s", req.Method)
		}
		rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
	})
	a2, events2 := newTestAdapterWithEvents(t, "", cancelRT)
	if err := a2.Cancel(context.Background(), dl); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	ev2 := <-events2
	if ev2.Type != downloader.EventCancelled || ev2.ID != dl.ID {
		t.Fatalf("cancel event: %#v", ev2)
	}
}

func TestAdapterDeleteDeletesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "a.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(file+".aria2", []byte("x"), 0o644); err != nil {
		t.Fatalf("write control: %v", err)
	}
	dl := &data.Download{ID: "1", GID: "gid1", TargetPath: tmpDir}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		switch call {
		case 1:
			if req.Method != "aria2.remove" {
				t.Fatalf("expected remove got %s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.removeDownloadResult" {
				t.Fatalf("expected removeDownloadResult got %s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("expected getFiles got %s", req.Method)
			}
			result := []map[string]any{{"path": file, "length": "1", "completedLength": "1"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected call %d", call)
			return nil, nil
		}
	})
	a := newTestAdapter(t, "", rt)
	fake := &fakeFS{}
	a.fs = fake
    if err := a.Delete(context.Background(), dl, true); err != nil {
        t.Fatalf("Delete: %v", err)
    }
    // Payload removed via RemoveAll; sidecar via Remove
    if len(fake.removedAll) != 1 || fake.removedAll[0] != file {
        t.Fatalf("unexpected RemoveAll calls: %#v", fake.removedAll)
    }
    if len(fake.removed) != 1 || fake.removed[0] != file+".aria2" {
        t.Fatalf("unexpected Remove calls: %#v", fake.removed)
    }
}

func TestAdapterDeletePrunesDirs(t *testing.T) {
	base := t.TempDir()
	// Create payload files and sidecars
	dir := filepath.Join(base, "Show", "Season 1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f1 := filepath.Join(dir, "E01.mkv")
	f2 := filepath.Join(dir, "E02.mkv")
	if err := os.WriteFile(f1, []byte("x"), 0o644); err != nil {
		t.Fatalf("write f1: %v", err)
	}
	if err := os.WriteFile(f1+".aria2", []byte("x"), 0o644); err != nil {
		t.Fatalf("write f1 sidecar: %v", err)
	}
	if err := os.WriteFile(f2, []byte("x"), 0o644); err != nil {
		t.Fatalf("write f2: %v", err)
	}
	if err := os.WriteFile(f2+".aria2", []byte("x"), 0o644); err != nil {
		t.Fatalf("write f2 sidecar: %v", err)
	}

	dl := &data.Download{ID: "1", GID: "gid1", TargetPath: base, Name: "Show", Source: "magnet:?xt=urn:btih:123"}

	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		switch call {
		case 1:
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			result := []map[string]any{{"path": f1, "length": "1", "completedLength": "1"}, {"path": f2, "length": "1", "completedLength": "1"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected call %d", call)
			return nil, nil
		}
	})

	a := newTestAdapter(t, "", rt)
	fake := &fakeFS{}
	a.fs = fake
	if err := a.Delete(context.Background(), dl, true); err != nil {
		t.Fatalf("Delete: %v", err)
	}

    expectedRemoved := map[string]struct{}{
        f1 + ".aria2":                           {},
        f2 + ".aria2":                           {},
        filepath.Join(base, "Show.aria2"):       {},
        filepath.Join(base, "Show.torrent"):     {},
        filepath.Join(base, "Show", "Season 1"): {},
        filepath.Join(base, "Show"):             {},
    }
    for _, p := range fake.removed {
        delete(expectedRemoved, p)
    }
    if len(expectedRemoved) != 0 {
        t.Fatalf("missing Remove calls: %#v", expectedRemoved)
    }
    expectedAll := map[string]struct{}{f1: {}, f2: {}}
    for _, p := range fake.removedAll {
        delete(expectedAll, p)
    }
    if len(expectedAll) != 0 {
        t.Fatalf("missing RemoveAll calls: %#v", expectedAll)
    }
}

func TestAdapterDeleteErrorPropagation(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "a.mkv")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	dl := &data.Download{ID: "1", TargetPath: base, Files: []data.DownloadFile{{Path: file}}}
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		if req.Method != "aria2.getFiles" {
			t.Fatalf("unexpected method %s", req.Method)
		}
		rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`[]`)})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
	})
	a := newTestAdapter(t, "", rt)
	fake := &fakeFS{errOn: map[string]error{file: fmt.Errorf("boom")}}
	a.fs = fake
	if err := a.Delete(context.Background(), dl, true); err == nil {
		t.Fatalf("expected error")
	}
}

type fakeFS struct {
	removed    []string
	removedAll []string
	errOn      map[string]error
}

func (f *fakeFS) Remove(p string) error {
	f.removed = append(f.removed, p)
	if f.errOn != nil {
		if err, ok := f.errOn[p]; ok {
			return err
		}
	}
	return nil
}
func (f *fakeFS) RemoveAll(p string) error {
	f.removedAll = append(f.removedAll, p)
	if f.errOn != nil {
		if err, ok := f.errOn[p]; ok {
			return err
		}
	}
	return nil
}

func TestAdapterDeleteSafety(t *testing.T) {
	tmpDir := t.TempDir()
	dl := &data.Download{ID: "1", GID: "gid1", TargetPath: tmpDir}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		switch call {
		case 1:
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			result := []map[string]any{{"path": "/etc/passwd", "length": "1", "completedLength": "1"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			return nil, nil
		}
	})
	a := newTestAdapter(t, "", rt)
	if err := a.Delete(context.Background(), dl, true); err == nil {
		t.Fatalf("expected error due to unsafe path")
	}
}

func TestAdapterPurgeSkipsSymlinkTargets(t *testing.T) {
	tmpDir := t.TempDir()

	outside := filepath.Join(tmpDir, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	outsideFile := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(outsideFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write outside: %v", err)
	}

	link := filepath.Join(tmpDir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.WriteFile(link+".aria2", []byte("x"), 0o644); err != nil {
		t.Fatalf("write control: %v", err)
	}

	dl := &data.Download{ID: "1", GID: "gid1", TargetPath: tmpDir}
	call := 0
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		call++
		b, _ := io.ReadAll(r.Body)
		var req rpcReq
		_ = json.Unmarshal(b, &req)
		switch call {
		case 1:
			if req.Method != "aria2.remove" {
				t.Fatalf("expected remove got %s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 2:
			if req.Method != "aria2.removeDownloadResult" {
				t.Fatalf("expected removeDownloadResult got %s", req.Method)
			}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: json.RawMessage(`"ok"`)})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		case 3:
			if req.Method != "aria2.getFiles" {
				t.Fatalf("expected getFiles got %s", req.Method)
			}
			result := []map[string]any{{"path": link, "length": "1", "completedLength": "1"}}
			rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Result: must(json.Marshal(result))})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
		default:
			t.Fatalf("unexpected call %d", call)
			return nil, nil
		}
	})
	a := newTestAdapter(t, "", rt)
	if err := a.Delete(context.Background(), dl, true); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink not removed")
	}
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("target directory affected: %v", err)
	}
}

func TestAdapterEmitsFilesMeta(t *testing.T) {
	dl := &data.Download{ID: "42", Source: "http://example.com/pack", TargetPath: "/tmp"}
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
	dl := &data.Download{ID: "7", Source: "http://example.com/onefile", TargetPath: "/tmp"}
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
	dl := &data.Download{ID: "11", Source: "magnet:?xt=urn:btih:abc&dn=My.Torrent", TargetPath: "/tmp"}
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
	dl := &data.Download{ID: "21", Source: "magnet:?xt=urn:btih:abc&dn=Z.Name", GID: "metaG"}
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

func TestAdapterStartConflictMapsErrConflict(t *testing.T) {
	dl := &data.Download{ID: "71", Source: "http://example.com/file.bin", TargetPath: "/tmp"}
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// addUri returns an RPC error that simulates a file-exists failure
		rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Error: &rpcError{Code: 1, Message: "File already exists"}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
	})
	a := newTestAdapter(t, "", rt)
	gid, err := a.Start(context.Background(), dl)
	if gid != "" {
		t.Fatalf("expected empty gid, got %q", gid)
	}
	if !errors.Is(err, data.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAdapterResumeConflictMapsErrConflict(t *testing.T) {
	dl := &data.Download{ID: "72", GID: "gid-72"}
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		// unpause returns conflict
		rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Error: &rpcError{Code: 1, Message: "File exists"}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
	})
	a := newTestAdapter(t, "", rt)
	err := a.Resume(context.Background(), dl)
	if !errors.Is(err, data.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAdapterCancelNotFoundMapsErrNotFound(t *testing.T) {
	dl := &data.Download{GID: "gid-missing"}
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		rb, _ := json.Marshal(rpcResp{Jsonrpc: "2.0", ID: "torrus", Error: &rpcError{Code: 1, Message: "GID not found"}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(rb)), Header: make(http.Header)}, nil
	})
	a := newTestAdapter(t, "", rt)
	err := a.Cancel(context.Background(), dl)
	if !errors.Is(err, downloader.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAdapterHandleNotification(t *testing.T) {
	events := make(chan downloader.Event, 2)
	rep := downloader.NewChanReporter(events)
	a := &Adapter{rep: rep, gidToID: map[string]string{"g1": "1", "g2": "2"}, activeGIDs: map[string]struct{}{}, lastProg: map[string]downloader.Progress{}}

	// Complete event
	a.handleNotification(context.Background(), aria2.Notification{Method: "aria2.onDownloadComplete", Params: []aria2.NotificationEvent{{GID: "g1"}}})
	ev := <-events
	if ev.Type != downloader.EventComplete || ev.ID != "1" || ev.GID != "g1" {
		t.Fatalf("unexpected event %#v", ev)
	}
	if _, ok := a.gidToID["g1"]; ok {
		t.Fatalf("gid not removed after complete")
	}

	// Error event
	a.handleNotification(context.Background(), aria2.Notification{Method: "aria2.onDownloadError", Params: []aria2.NotificationEvent{{GID: "g2"}}})
	ev = <-events
	if ev.Type != downloader.EventFailed || ev.ID != "2" || ev.GID != "g2" {
		t.Fatalf("unexpected event %#v", ev)
	}
}

// --- Consolidated delete fallback tests ---

func TestDelete_HTTPFile_Cancelled_Fallback_RemovesFileAndAria2(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    base := t.TempDir()
    file := filepath.Join(base, "movie.mp4")
    if err := os.WriteFile(file, []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(file+".aria2", []byte("m"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "id1", Source: "https://example.com/movie.mp4", TargetPath: base, Name: "movie.mp4"}
    a := newAdapterNoRPC(t)

    if err := a.Delete(ctx, dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    if _, err := os.Stat(file); !os.IsNotExist(err) { t.Fatalf("payload not deleted: %v", err) }
    if _, err := os.Stat(file+".aria2"); !os.IsNotExist(err) { t.Fatalf("aria2 sidecar not deleted: %v", err) }
}

func TestDelete_TorrentFolder_Cancelled_RemovesFolderAndSidecars(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    base := t.TempDir()
    root := filepath.Join(base, "MyTorrent")
    nested := filepath.Join(root, "disc1")
    if err := os.MkdirAll(nested, 0o755); err != nil { t.Fatal(err) }
    p := filepath.Join(nested, "track.flac")
    if err := os.WriteFile(p, []byte("x"), 0o644); err != nil { t.Fatal(err) }
    // sidecars
    if err := os.WriteFile(root+".aria2", []byte("a"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(base, "MyTorrent.torrent"), []byte("t"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(root+".torrent", []byte("t2"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "id2", Source: "magnet:?xt=urn:btih:abc", TargetPath: base, Name: "MyTorrent"}
    a := newAdapterNoRPC(t)

    if err := a.Delete(ctx, dl, true); err != nil { t.Fatalf("Delete: %v", err) }

    if _, err := os.Stat(root); !os.IsNotExist(err) { t.Fatalf("folder not deleted: %v", err) }
    if _, err := os.Stat(root+".aria2"); !os.IsNotExist(err) { t.Fatalf("root .aria2 not deleted: %v", err) }
    if _, err := os.Stat(filepath.Join(base, "MyTorrent.torrent")); !os.IsNotExist(err) { t.Fatalf(".torrent sidecar not deleted: %v", err) }
    if _, err := os.Stat(root+".torrent"); !os.IsNotExist(err) { t.Fatalf("root .torrent not deleted: %v", err) }
}

func TestDelete_EarlyCancel_NoFiles_GIDGone_UsesFallback(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    base := t.TempDir()
    root := filepath.Join(base, "Early")
    if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil { t.Fatal(err) }
    f := filepath.Join(root, "sub", "a.bin")
    if err := os.WriteFile(f, []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(root+".aria2", []byte("a"), 0o644); err != nil { t.Fatal(err) }

    dl := &data.Download{ID: "id3", Source: "https://host/early", TargetPath: base, Name: "Early"}
    a := newAdapterNoRPC(t)

    if err := a.Delete(ctx, dl, true); err != nil { t.Fatalf("Delete: %v", err) }
    if _, err := os.Stat(root); !os.IsNotExist(err) { t.Fatalf("root dir not removed: %v", err) }
}

func TestDelete_TrimmedLeadingTag_SidecarRemoved(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    base := t.TempDir()
    // Real on-disk folder has trailing tag but no leading [METADATA]
    real := "Rick.and.Morty.S07E03.[TGx]"
    root := filepath.Join(base, real)
    if err := os.MkdirAll(filepath.Join(root, "d"), 0o755); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(root, "d", "f"), []byte("x"), 0o644); err != nil { t.Fatal(err) }
    // Root control sidecar placed at TargetPath
    if err := os.WriteFile(filepath.Join(base, real+".aria2"), []byte("a"), 0o644); err != nil { t.Fatal(err) }

    // Name from metadata includes a leading tag that should be trimmed
    dl := &data.Download{ID: "idX", Source: "magnet:?xt=urn:btih:xyz", TargetPath: base, Name: "[METADATA] "+real}
    a := newAdapterNoRPC(t)
    if err := a.Delete(ctx, dl, true); err != nil { t.Fatalf("Delete: %v", err) }
    if _, err := os.Stat(root); !os.IsNotExist(err) { t.Fatalf("root dir not removed: %v", err) }
    if _, err := os.Stat(filepath.Join(base, real+".aria2")); !os.IsNotExist(err) { t.Fatalf("root sidecar not removed: %v", err) }
}

func TestDelete_TrimmedRoot_NoSidecar_OwnershipByFiles(t *testing.T) {
    t.Parallel()
    ctx := context.Background()
    base := t.TempDir()
    real := "Show.S01.[TGx]"
    root := filepath.Join(base, real)
    if err := os.MkdirAll(filepath.Join(root, "s"), 0o755); err != nil { t.Fatal(err) }
    fname1 := "E01.mkv"
    fname2 := "E02.srt"
    if err := os.WriteFile(filepath.Join(root, "s", fname1), []byte("x"), 0o644); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(root, "s", fname2), []byte("y"), 0o644); err != nil { t.Fatal(err) }

    // No root .aria2 sidecar present; rely on file ownership check
    dl := &data.Download{ID: "idY", Source: "magnet:?xt=urn:btih:xyz", TargetPath: base, Name: "[METADATA] "+real,
        Files: []data.DownloadFile{{Path: fname1}, {Path: fname2}}}
    a := newAdapterNoRPC(t)
    if err := a.Delete(ctx, dl, true); err != nil { t.Fatalf("Delete: %v", err) }
    if _, err := os.Stat(root); !os.IsNotExist(err) { t.Fatalf("root dir not removed: %v", err) }
}

func TestAdapterMetadataCompleteTriggersFollowedBySwap(t *testing.T) {
	// Start with a magnet where immediate followedBy is empty; later a completion
	// notification for the metadata gid should cause a swap to the real gid.
	dl := &data.Download{ID: "33", Source: "magnet:?xt=urn:btih:xyz&dn=Title", TargetPath: "/tmp"}
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

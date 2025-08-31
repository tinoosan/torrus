package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	internaldata "github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/downloader"
	"github.com/tinoosan/torrus/internal/repo"
	"github.com/tinoosan/torrus/internal/router"
	"github.com/tinoosan/torrus/internal/service"
)

const testToken = "testtoken"

func setup(t *testing.T) http.Handler {
	t.Helper()
	t.Setenv("TORRUS_API_TOKEN", testToken)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repo.NewInMemoryDownloadRepo()
	dlr := downloader.NewNoopDownloader()
	svc := service.NewDownload(repo, dlr)
	return router.New(logger, svc)
}

func authReq(r *http.Request) {
	r.Header.Set("Authorization", "Bearer "+testToken)
}

func TestHealthz(t *testing.T) {
	h := setup(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "ok" {
		t.Fatalf("expected body 'ok' got %q", rr.Body.String())
	}
}

func TestDownloadsLifecycle(t *testing.T) {
	h := setup(t)

	// GET empty list
	req := httptest.NewRequest(http.MethodGet, "/v1/downloads", nil)
	authReq(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d", rr.Code)
	}
	var list []map[string]any
	err := json.NewDecoder(rr.Body).Decode(&list)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list got %v", list)
	}

	// POST valid download
	body := bytes.NewBufferString(`{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp/file"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/downloads", body)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201 got %d", rr.Code)
	}
	var created map[string]any
	err = json.NewDecoder(rr.Body).Decode(&created)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	id := int(created["id"].(float64))

	// GET list should have one item
	req = httptest.NewRequest(http.MethodGet, "/v1/downloads", nil)
	authReq(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d", rr.Code)
	}
	list = nil
	err = json.NewDecoder(rr.Body).Decode(&list)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || int(list[0]["id"].(float64)) != id {
		t.Fatalf("unexpected list: %v", list)
	}

	// GET existing download
	req = httptest.NewRequest(http.MethodGet, "/v1/downloads/"+strconv.Itoa(id), nil)
	authReq(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 got %d", rr.Code)
	}

	// GET missing download
	req = httptest.NewRequest(http.MethodGet, "/v1/downloads/9999", nil)
	authReq(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 got %d", rr.Code)
	}
}

func TestPostIdempotent(t *testing.T) {
	h := setup(t)

	body := bytes.NewBufferString(`{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp/file"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/downloads", body)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var first map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&first)

	// Same request again => 200 and same id
	body2 := bytes.NewBufferString(`{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp/file"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/downloads", body2)
	authReq(req2)
	req2.Header.Set("Content-Type", "application/json")
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr2.Code)
	}
	var second map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&second)
	if int(first["id"].(float64)) != int(second["id"].(float64)) {
		t.Fatalf("ids differ: %v vs %v", first["id"], second["id"])
	}
}

func TestPostDownloadValidation(t *testing.T) {
	h := setup(t)

	tests := []struct {
		name        string
		contentType string
		body        string
		want        int
	}{
		{"wrong content-type", "text/plain", "{}", http.StatusUnsupportedMediaType},
		{"unknown field", "application/json", `{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp","extra":1}`, http.StatusBadRequest},
		{"missing target", "application/json", `{"source":"magnet:?xt=urn:btih:abcdef"}`, http.StatusBadRequest},
		{"body too large", "application/json", `{"source":"magnet:?xt=urn:btih:` + strings.Repeat("a", 1<<20) + `","targetPath":"/tmp"}`, http.StatusBadRequest},
		{"name provided (read-only)", "application/json", `{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp","name":"hack"}`, http.StatusBadRequest},
		{"files provided (read-only)", "application/json", `{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp","files":[{"path":"a.mkv"}]}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/downloads", strings.NewReader(tt.body))
			authReq(req)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Fatalf("expected status %d got %d", tt.want, rr.Code)
			}
		})
	}
}

func TestDeleteDownload(t *testing.T) {
	h := setup(t)

	// create a download first
	body := bytes.NewBufferString(`{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp/file"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/downloads", body)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	id := int(created["id"].(float64))

	// delete without body
	delReq := httptest.NewRequest(http.MethodDelete, "/v1/downloads/"+strconv.Itoa(id), nil)
	authReq(delReq)
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", delRR.Code)
	}

	// deleting again should return 404
	delReq2 := httptest.NewRequest(http.MethodDelete, "/v1/downloads/"+strconv.Itoa(id), nil)
	authReq(delReq2)
	delRR2 := httptest.NewRecorder()
	h.ServeHTTP(delRR2, delReq2)
	if delRR2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", delRR2.Code)
	}

	// create another download for body tests
	body = bytes.NewBufferString(`{"source":"magnet:?xt=urn:btih:aaaa","targetPath":"/tmp/file2"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/downloads", body)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}
	_ = json.NewDecoder(rr.Body).Decode(&created)
	id2 := int(created["id"].(float64))

	// delete with deleteFiles true
	delBody := bytes.NewBufferString(`{"deleteFiles":true}`)
	delReq3 := httptest.NewRequest(http.MethodDelete, "/v1/downloads/"+strconv.Itoa(id2), delBody)
	authReq(delReq3)
	delReq3.Header.Set("Content-Type", "application/json")
	delRR3 := httptest.NewRecorder()
	h.ServeHTTP(delRR3, delReq3)
	if delRR3.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d", delRR3.Code)
	}

	// bad json
	badReq := httptest.NewRequest(http.MethodDelete, "/v1/downloads/1", strings.NewReader("{"))
	authReq(badReq)
	badReq.Header.Set("Content-Type", "application/json")
	badRR := httptest.NewRecorder()
	h.ServeHTTP(badRR, badReq)
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", badRR.Code)
	}

	// wrong content-type
	ctReq := httptest.NewRequest(http.MethodDelete, "/v1/downloads/1", strings.NewReader("{}"))
	authReq(ctReq)
	ctReq.Header.Set("Content-Type", "text/plain")
	ctRR := httptest.NewRecorder()
	h.ServeHTTP(ctRR, ctReq)
	if ctRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", ctRR.Code)
	}
}

func TestGetDownloadIncludesFiles(t *testing.T) {
	// Build router manually to access repo
	t.Setenv("TORRUS_API_TOKEN", testToken)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rpo := repo.NewInMemoryDownloadRepo()
	dlr := downloader.NewNoopDownloader()
	svc := service.NewDownload(rpo, dlr)
	h := router.New(logger, svc)

	// Seed a download with files
	dl := &struct {
		Source     string `json:"source"`
		TargetPath string `json:"targetPath"`
	}{"magnet:?xt=urn:btih:abcdef", "/tmp"}

	// Create download via API
	b := new(bytes.Buffer)
	_ = json.NewEncoder(b).Encode(dl)
	req := httptest.NewRequest(http.MethodPost, "/v1/downloads", b)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d", rr.Code)
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	id := int(created["id"].(float64))

	// Update repo to include files
	_, _ = rpo.Update(context.Background(), id, func(d *internaldata.Download) error {
		d.Files = []internaldata.DownloadFile{{Path: "ep1.mkv", Length: 1000}, {Path: "ep2.mkv", Completed: 100}}
		return nil
	})

	// GET by id should include files
	req = httptest.NewRequest(http.MethodGet, "/v1/downloads/"+strconv.Itoa(id), nil)
	authReq(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d", rr.Code)
	}
	var got map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&got)
	fs, ok := got["files"].([]any)
	if !ok || len(fs) != 2 {
		t.Fatalf("files missing or wrong len: %#v", got["files"])
	}
}

func TestPatchDownload(t *testing.T) {
	h := setup(t)

	// first create a download
	body := bytes.NewBufferString(`{"source":"magnet:?xt=urn:btih:abcdef","targetPath":"/tmp/file"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/downloads", body)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201 got %d", rr.Code)
	}

	tests := []struct {
		name        string
		url         string
		contentType string
		body        string
		want        int
	}{
		{"valid", "/v1/downloads/1", "application/json", `{"desiredStatus":"Paused"}`, http.StatusOK},
		{"invalid status", "/v1/downloads/1", "application/json", `{"desiredStatus":"Bad"}`, http.StatusBadRequest},
		{"unknown id", "/v1/downloads/999", "application/json", `{"desiredStatus":"Paused"}`, http.StatusNotFound},
		{"wrong content-type", "/v1/downloads/1", "text/plain", `{"desiredStatus":"Paused"}`, http.StatusUnsupportedMediaType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, tt.url, strings.NewReader(tt.body))
			authReq(req)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Fatalf("expected status %d got %d", tt.want, rr.Code)
			}
		})
	}
}

type conflictDL struct{}

func (c *conflictDL) Start(ctx context.Context, d *internaldata.Download) (string, error) {
	return "", internaldata.ErrConflict
}
func (c *conflictDL) Pause(ctx context.Context, d *internaldata.Download) error { return nil }
func (c *conflictDL) Resume(ctx context.Context, d *internaldata.Download) error {
	return internaldata.ErrConflict
}
func (c *conflictDL) Cancel(ctx context.Context, d *internaldata.Download) error { return nil }
func (c *conflictDL) Purge(ctx context.Context, d *internaldata.Download) error  { return nil }

func TestPatchConflictPolicyReturns409(t *testing.T) {
	t.Setenv("TORRUS_API_TOKEN", testToken)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rpo := repo.NewInMemoryDownloadRepo()
	dlr := &conflictDL{}
	svc := service.NewDownload(rpo, dlr)
	h := router.New(logger, svc)

	// Create download via API
	body := bytes.NewBufferString(`{"source":"http://example.com/file.bin","targetPath":"/tmp"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/downloads", body)
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status=%d", rr.Code)
	}

	// Now PATCH desiredStatus Active -> should hit Start and return 409
	req = httptest.NewRequest(http.MethodPatch, "/v1/downloads/1", strings.NewReader(`{"desiredStatus":"Active"}`))
	authReq(req)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
}

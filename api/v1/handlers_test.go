package v1_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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

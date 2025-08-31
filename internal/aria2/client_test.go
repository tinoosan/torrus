package aria2

import (
	"os"
	"testing"
	"time"
)

func TestNewClientFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		secret      string
		timeoutMS   string
		wantURL     string
		wantSecret  string
		wantTimeout time.Duration
	}{
		{
			name:        "defaults",
			wantURL:     "http://127.0.0.1:6800/jsonrpc",
			wantSecret:  "",
			wantTimeout: 3 * time.Second,
		},
		{
			name:        "valid env values",
			url:         "http://localhost:6801/jsonrpc",
			secret:      "token:abc123",
			timeoutMS:   "1500",
			wantURL:     "http://localhost:6801/jsonrpc",
			wantSecret:  "token:abc123",
			wantTimeout: 1500 * time.Millisecond,
		},
		{
			name:        "invalid url fallback",
			url:         "::bad::url",
			wantURL:     "http://127.0.0.1:6800/jsonrpc",
			wantSecret:  "",
			wantTimeout: 3 * time.Second,
		},
		{
			name:        "invalid timeout string",
			url:         "http://localhost:6801/jsonrpc",
			timeoutMS:   "not-a-number",
			wantURL:     "http://localhost:6801/jsonrpc",
			wantSecret:  "",
			wantTimeout: 3 * time.Second,
		},
		{
			name:        "negative timeout",
			timeoutMS:   "-25",
			wantURL:     "http://127.0.0.1:6800/jsonrpc",
			wantSecret:  "",
			wantTimeout: 3000 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// ensure clean environment and set using t.Setenv
			for _, k := range []string{"ARIA2_RPC_URL", "ARIA2_SECRET", "ARIA2_TIMEOUT_MS"} {
				err := os.Unsetenv(k)
				if err != nil {
					t.Fatalf("unset %s: %v", k, err)
				}
			}

			t.Setenv("ARIA2_RPC_URL", tc.url)
			t.Setenv("ARIA2_SECRET", tc.secret)
			t.Setenv("ARIA2_TIMEOUT_MS", tc.timeoutMS)

			c, err := NewClientFromEnv()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := c.baseURL.String(); got != tc.wantURL {
				t.Fatalf("url: got %q want %q", got, tc.wantURL)
			}
			if c.secret != tc.wantSecret {
				t.Fatalf("secret: got %q want %q", c.secret, tc.wantSecret)
			}
			if c.http == nil {
				t.Fatalf("http client is nil")
			}
			if c.http.Timeout != tc.wantTimeout {
				t.Fatalf("timeout: got %v want %v", c.http.Timeout, tc.wantTimeout)
			}
			if tc.name == "negative timeout" {
				t.Logf("TODO: clamp negative timeout to a minimum positive value")
			}
		})
	}
}

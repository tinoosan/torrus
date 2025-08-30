package aria2

import (
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Client struct {
	baseURL *url.URL
	secret  string
	http    *http.Client
}

func NewClientFromEnv() (*Client, error) {

	ms, err3 := strconv.Atoi(os.Getenv("ARIA2_TIMEOUT_MS"))
	if err3 != nil || ms <= 0 {
		ms = 3000
	}
	secret := os.Getenv("ARIA2_SECRET")

	rawURL := os.Getenv("ARIA_RPC_URL")

	if rawURL == "" {
		rawURL = "http://127.0.0.1:6800/jsonrpc"
	}

	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		http:    &http.Client{Timeout: time.Duration(ms) * time.Millisecond},
	}, nil
}

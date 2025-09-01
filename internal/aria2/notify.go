package aria2

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nhooyr.io/websocket"
)

// Notification represents an async event pushed by aria2.
type Notification struct {
	Method string              `json:"method"`
	Params []NotificationEvent `json:"params"`
}

// NotificationEvent contains details for an aria2 notification.
type NotificationEvent struct {
	GID string `json:"gid"`
}

// Notifications connects to the aria2 WebSocket endpoint and streams
// async notifications. The returned channel is closed when the connection
// terminates or the context is cancelled.
func (c *Client) Notifications(ctx context.Context) (<-chan Notification, error) {
	wsURL := *c.baseURL
	switch wsURL.Scheme {
	case "http":
		wsURL.Scheme = "ws"
	case "https":
		wsURL.Scheme = "wss"
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", wsURL.Scheme)
	}
	// nhooyr websocket requires no fragments; path remains same
	conn, _, err := websocket.Dial(ctx, wsURL.String(), nil)
	if err != nil {
		return nil, err
	}
	ch := make(chan Notification, 8)
	go func() {
		defer close(ch)
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "done") }()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			// aria2 may send newline-delimited JSON; trim
			data = []byte(strings.TrimSpace(string(data)))
			var n Notification
			if err := json.Unmarshal(data, &n); err != nil {
				continue
			}
			ch <- n
		}
	}()
	return ch, nil
}

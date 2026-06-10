package transport

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Hijack POSTs body to path with Connection: Upgrade / Upgrade: tcp, reads the
// 101 response, and returns the raw connection (positioned at the start of the
// stream body) plus the response headers (which carry Sandboxes-Exec-Id).
// The caller owns conn and must Close it.
func (t *Transport) Hijack(ctx context.Context, path string, jsonBody []byte) (net.Conn, http.Header, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", t.socket)
	if err != nil {
		return nil, nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	req := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: sandboxd\r\n"+
		"User-Agent: sbx-go-sdk\r\nContent-Type: application/json\r\n"+
		"Connection: Upgrade\r\nUpgrade: tcp\r\nContent-Length: %d\r\n\r\n",
		path, len(jsonBody))
	if _, err := conn.Write(append([]byte(req), jsonBody...)); err != nil {
		conn.Close()
		return nil, nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodPost})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Body.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("attach: expected 101, got %d", resp.StatusCode)
	}
	// Reset the deadline so the stream isn't killed by the handshake deadline.
	conn.SetDeadline(time.Time{})
	// bufio may have buffered stream bytes after the headers; wrap so they aren't lost.
	return &bufferedConn{Conn: conn, r: br}, resp.Header, nil
}

// bufferedConn returns any bytes the header reader already buffered before
// falling through to the underlying conn.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }

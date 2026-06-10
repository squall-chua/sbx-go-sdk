package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Transport is an HTTP client bound to a single unix-domain socket.
type Transport struct {
	socket string
	hc     *http.Client
}

// New returns a Transport that dials the given unix socket for every request.
func New(socket string) *Transport {
	return &Transport{
		socket: socket,
		hc: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
				DisableCompression: true,
			},
		},
	}
}

// SetTimeout sets the per-request timeout (0 = none).
func (t *Transport) SetTimeout(d time.Duration) { t.hc.Timeout = d }

// Socket returns the unix socket path this transport dials.
func (t *Transport) Socket() string { return t.socket }

// Do issues an HTTP request. The URL host is a placeholder ("sandboxd"); only the
// path matters because every connection goes to the bound socket.
func (t *Transport) Do(ctx context.Context, method, path string, body io.Reader, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://sandboxd"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sbx-go-sdk")
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return t.hc.Do(req)
}

// DoJSON sends an optional JSON body and decodes a JSON response into out (if non-nil).
// It returns the raw status and body to the caller via error on non-2xx.
func (t *Transport) DoJSON(ctx context.Context, method, path string, in, out any) error {
	var rdr io.Reader
	hdr := http.Header{}
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
		hdr.Set("Content-Type", "application/json")
	}
	resp, err := t.Do(ctx, method, path, rdr, hdr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{Status: resp.StatusCode, Body: data}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// HTTPStatusError carries a non-2xx response for the client layer to map to typed errors.
type HTTPStatusError struct {
	Status int
	Body   []byte
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("sbx daemon returned HTTP %d: %s", e.Status, bytes.TrimSpace(e.Body))
}

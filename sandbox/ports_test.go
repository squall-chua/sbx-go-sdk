package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPorts_ListAndPublish(t *testing.T) {
	var published []Port
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sandbox/s1/ports", r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`[{"host_ip":"127.0.0.1","host_port":18080,"protocol":"tcp","sandbox_port":8080}]`))
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &published))
			w.Write([]byte(`[{"host_ip":"127.0.0.1","host_port":19090,"protocol":"tcp","sandbox_port":9090}]`))
		}
	}))
	sb := NewForTest(c, "s1")

	ports, err := sb.Ports(context.Background())
	require.NoError(t, err)
	require.Len(t, ports, 1)
	require.Equal(t, 8080, ports[0].SandboxPort)
	require.Equal(t, 18080, ports[0].HostPort)

	_, err = sb.PublishPort(context.Background(), Port{SandboxPort: 9090, HostPort: 19090, HostIP: "127.0.0.1", Protocol: "tcp"})
	require.NoError(t, err)
	require.Len(t, published, 1)
	require.Equal(t, 9090, published[0].SandboxPort)
}

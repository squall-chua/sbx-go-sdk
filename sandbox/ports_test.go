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

func TestUnpublishPort_ExpandsBothAddressFamilies(t *testing.T) {
	// Publishing "18080:8080" creates a 127.0.0.1 key AND a ::1 key, so a spec
	// without a host address must release both.
	var sent []Port
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1/ports":
			w.Write([]byte(`[
				{"host_ip":"127.0.0.1","host_port":18080,"protocol":"tcp","sandbox_port":8080},
				{"host_ip":"::1","host_port":18080,"protocol":"tcp","sandbox_port":8080},
				{"host_ip":"127.0.0.1","host_port":19090,"protocol":"tcp","sandbox_port":9090}
			]`))
		case "/sandbox/s1/ports/unpublish":
			require.Equal(t, http.MethodPost, r.Method)
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &sent), "body must be a bare array")
			w.Write([]byte(`{"message":"unpublished 2 port key(s)"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	sb := NewForTest(c, "s1")

	require.NoError(t, sb.UnpublishPort(context.Background(), "18080:8080"))
	require.Len(t, sent, 2, "both address families must be unpublished")
	require.ElementsMatch(t, []string{"127.0.0.1", "::1"}, []string{sent[0].HostIP, sent[1].HostIP})
	for _, p := range sent {
		require.Equal(t, 8080, p.SandboxPort)
	}
}

func TestUnpublishPort_ExplicitHostIPSendsOneKey(t *testing.T) {
	var sent []Port
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1/ports":
			w.Write([]byte(`[
				{"host_ip":"127.0.0.1","host_port":18080,"protocol":"tcp","sandbox_port":8080},
				{"host_ip":"::1","host_port":18080,"protocol":"tcp","sandbox_port":8080}
			]`))
		case "/sandbox/s1/ports/unpublish":
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &sent))
			w.Write([]byte(`{"message":"unpublished 1 port key(s)"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	sb := NewForTest(c, "s1")

	require.NoError(t, sb.UnpublishPort(context.Background(), "127.0.0.1:18080:8080/tcp"))
	require.Len(t, sent, 1, "the explicit host IP must narrow to exactly one key")
	require.Equal(t, "127.0.0.1", sent[0].HostIP)
	require.Equal(t, 18080, sent[0].HostPort)
	require.Equal(t, 8080, sent[0].SandboxPort)
	require.Equal(t, "tcp", sent[0].Protocol)
}

func TestUnpublishPort_ExplicitHostIPWithoutProtocolStillMatches(t *testing.T) {
	// "127.0.0.1:18080:8080" gives no protocol, so posting it directly would
	// send protocol:"" against a published key carrying "tcp" and silently
	// no-op. Filtering against the published list instead means the daemon's
	// protocol is used.
	var sent []Port
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1/ports":
			w.Write([]byte(`[{"host_ip":"127.0.0.1","host_port":18080,"protocol":"tcp","sandbox_port":8080}]`))
		case "/sandbox/s1/ports/unpublish":
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &sent))
			w.Write([]byte(`{"message":"unpublished 1 port key(s)"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	sb := NewForTest(c, "s1")

	require.NoError(t, sb.UnpublishPort(context.Background(), "127.0.0.1:18080:8080"))
	require.Len(t, sent, 1)
	require.Equal(t, "tcp", sent[0].Protocol, "the daemon's protocol must be used, not an empty string")
}

func TestUnpublishPort_InvalidSpecIsRejectedBeforeAnyRequest(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("no request expected for an invalid spec, got %s", r.URL.Path)
	}))
	sb := NewForTest(c, "s1")
	require.Error(t, sb.UnpublishPort(context.Background(), "not-a-spec"))
}

func TestUnpublishPort_NoMatchingPublishedPort(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sandbox/s1/ports" {
			w.Write([]byte(`[]`))
			return
		}
		t.Fatalf("must not post an empty key set: %s", r.URL.Path)
	}))
	sb := NewForTest(c, "s1")
	require.Error(t, sb.UnpublishPort(context.Background(), "18080:8080"))
}

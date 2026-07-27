package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePortSpec(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec string
		want Port
	}{
		{"host and sandbox port", "18080:8080", Port{HostPort: 18080, SandboxPort: 8080}},
		{"with protocol", "18080:8080/udp", Port{HostPort: 18080, SandboxPort: 8080, Protocol: "udp"}},
		{"with host ip", "127.0.0.1:18080:8080", Port{HostIP: "127.0.0.1", HostPort: 18080, SandboxPort: 8080}},
		{"host ip and protocol", "127.0.0.1:18080:8080/tcp", Port{HostIP: "127.0.0.1", HostPort: 18080, SandboxPort: 8080, Protocol: "tcp"}},
		{"ipv6 bracketed", "[::1]:18080:8080/tcp", Port{HostIP: "::1", HostPort: 18080, SandboxPort: 8080, Protocol: "tcp"}},
		{"ipv6 bracketed no protocol", "[::1]:18080:8080", Port{HostIP: "::1", HostPort: 18080, SandboxPort: 8080}},
		{"sandbox port only", "8080", Port{SandboxPort: 8080}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePortSpec(tc.spec)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParsePortSpec_Invalid(t *testing.T) {
	for _, spec := range []string{
		"", "abc", "18080:", ":8080", "18080:8080/", "a:b:c:d:e",
		"[::1:18080:8080", "18080:notaport",
		"[]:1:2", "127.0.0.1]:18080:8080", "18080:8080/tc:p", "18080:8080/TCP",
		"notanip:18080:8080",
	} {
		t.Run(spec, func(t *testing.T) {
			_, err := parsePortSpec(spec)
			require.Error(t, err, "spec %q must be rejected", spec)
		})
	}
}

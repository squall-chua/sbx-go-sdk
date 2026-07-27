package sandbox

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// parsePortSpec parses the CLI port spec form used by `sbx ports --unpublish`:
//
//	[[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]
//
// An IPv6 host address is bracketed, e.g. "[::1]:18080:8080/tcp".
func parsePortSpec(spec string) (Port, error) {
	var p Port
	s := strings.TrimSpace(spec)
	if s == "" {
		return p, fmt.Errorf("port spec is empty")
	}

	// Trailing /PROTOCOL.
	if i := strings.LastIndex(s, "/"); i >= 0 {
		p.Protocol = s[i+1:]
		s = s[:i]
		if p.Protocol == "" {
			return Port{}, fmt.Errorf("port spec %q: empty protocol", spec)
		}
		for i := 0; i < len(p.Protocol); i++ {
			if p.Protocol[i] < 'a' || p.Protocol[i] > 'z' {
				return Port{}, fmt.Errorf("port spec %q: invalid protocol %q", spec, p.Protocol)
			}
		}
	}

	// Leading [IPv6].
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 {
			return Port{}, fmt.Errorf("port spec %q: unterminated IPv6 address", spec)
		}
		p.HostIP = s[1:end]
		if net.ParseIP(p.HostIP) == nil {
			return Port{}, fmt.Errorf("port spec %q: invalid IPv6 address %q", spec, p.HostIP)
		}
		rest := strings.TrimPrefix(s[end+1:], ":")
		if rest == s[end+1:] {
			return Port{}, fmt.Errorf("port spec %q: expected ':' after IPv6 address", spec)
		}
		s = rest
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 1:
		// SANDBOX_PORT
	case 2:
		// HOST_PORT:SANDBOX_PORT
	case 3:
		// HOST_IP:HOST_PORT:SANDBOX_PORT (IPv4 form)
		if p.HostIP != "" {
			return Port{}, fmt.Errorf("port spec %q: host address given twice", spec)
		}
		p.HostIP, parts = parts[0], parts[1:]
		if net.ParseIP(p.HostIP) == nil {
			return Port{}, fmt.Errorf("port spec %q: invalid host address %q", spec, p.HostIP)
		}
	default:
		return Port{}, fmt.Errorf("port spec %q: too many ':'-separated fields", spec)
	}

	sandboxPart := parts[len(parts)-1]
	sp, err := parsePortNumber(sandboxPart)
	if err != nil {
		return Port{}, fmt.Errorf("port spec %q: sandbox port: %w", spec, err)
	}
	p.SandboxPort = sp

	if len(parts) == 2 {
		hp, err := parsePortNumber(parts[0])
		if err != nil {
			return Port{}, fmt.Errorf("port spec %q: host port: %w", spec, err)
		}
		p.HostPort = hp
	}
	if p.HostIP != "" && p.HostPort == 0 {
		return Port{}, fmt.Errorf("port spec %q: host address requires a host port", spec)
	}
	return p, nil
}

func parsePortNumber(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("missing port")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port %d out of range", n)
	}
	return n, nil
}

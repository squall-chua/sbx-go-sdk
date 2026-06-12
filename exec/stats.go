package exec

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// statsScript reads the same metrics the sbx TUI dashboard shows — nproc,
// /proc/meminfo, /proc/uptime, df -BG /, /proc/stat — but emits the core
// CPU/memory samples FIRST so a flaky df (busybox without -BG, or a wrapped long
// device name) can only zero the trailing disk/uptime fields, never sink the
// snapshot. The two /proc/stat reads bracket a 200ms sleep so CPU% comes from one
// exec; the TUI instead diffs /proc/stat across its refresh ticks.
const statsScript = "nproc 2>/dev/null; " +
	"awk '/^MemTotal/{print $2}/^MemAvailable/{print $2}' /proc/meminfo 2>/dev/null; " +
	"head -1 /proc/stat 2>/dev/null; sleep 0.2; head -1 /proc/stat 2>/dev/null; " +
	"awk '{print $1}' /proc/uptime 2>/dev/null; " +
	"df -BG / 2>/dev/null | awk 'NR==2{print $2,$3}'"

// Stats runs a short probe inside the sandbox and returns a resource-usage
// snapshot — the same metrics the sbx TUI shows. The sandbox must be running, or
// pass WithAutoStart to start a stopped one.
//
// CPUPercent is sampled over a ~200ms window, so the call blocks for at least
// that long and needs coreutils (nproc, fractional sleep) — with a non-fractional
// sleep it still returns but CPUPercent reads 0. UptimeSeconds and the Disk*
// fields are best-effort: they read 0 if the sandbox can't supply them (e.g. a
// busybox df without -BG), without failing the CPU/memory snapshot.
func Stats(ctx context.Context, sb *sandbox.Sandbox, opts ...ProcessOption) (Usage, error) {
	code, r, err := Exec(ctx, sb, []string{"sh", "-c", statsScript}, opts...)
	if err != nil {
		return Usage{}, err
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return Usage{}, client.MapError("stats", err)
	}
	if code != 0 {
		return Usage{}, fmt.Errorf("stats: probe exited %d: %q", code, out)
	}
	return parseStats(out)
}

// Usage is a point-in-time resource snapshot of a sandbox, read from /proc and
// df inside the sandbox — the same metrics the sbx TUI dashboard shows. Memory
// values are in kibibytes (as /proc/meminfo reports); disk values are in
// gigabytes (df -BG, for the root filesystem).
type Usage struct {
	Cores          int     // online CPUs (nproc)
	MemTotalKB     uint64  // /proc/meminfo MemTotal
	MemAvailableKB uint64  // /proc/meminfo MemAvailable
	MemUsedKB      uint64  // MemTotal - MemAvailable
	CPUPercent     float64 // mean utilization across all cores over the sample window, clamped 0..100
	UptimeSeconds  float64 // /proc/uptime, seconds since boot (0 if unavailable)
	DiskTotalGB    float64 // df -BG / size, gigabytes (0 if unavailable)
	DiskUsedGB     float64 // df -BG / used, gigabytes (0 if unavailable)
}

// parseStats parses the probe output in script order: nproc, MemTotal,
// MemAvailable, two /proc/stat aggregate lines sampled a short interval apart,
// then the best-effort uptime and "df total/used" lines. CPU% is the busy
// fraction over the sample interval. The trailing uptime/disk lines are optional
// — a missing or flaky df zeroes only those fields, never the core snapshot.
func parseStats(raw []byte) (Usage, error) {
	lines := nonEmptyLines(raw)
	if len(lines) < 5 {
		return Usage{}, fmt.Errorf("stats: unexpected probe output (%d lines): %q", len(lines), raw)
	}

	cores, err := strconv.Atoi(lines[0])
	if err != nil {
		return Usage{}, fmt.Errorf("stats: parsing nproc %q: %w", lines[0], err)
	}
	memTotal, err := strconv.ParseUint(lines[1], 10, 64)
	if err != nil {
		return Usage{}, fmt.Errorf("stats: parsing MemTotal %q: %w", lines[1], err)
	}
	memAvail, err := strconv.ParseUint(lines[2], 10, 64)
	if err != nil {
		return Usage{}, fmt.Errorf("stats: parsing MemAvailable %q: %w", lines[2], err)
	}

	a, err := parseCPULine(lines[3])
	if err != nil {
		return Usage{}, err
	}
	b, err := parseCPULine(lines[4])
	if err != nil {
		return Usage{}, err
	}

	// Deltas in float64 (not uint64) so a non-monotonic counter — iowait can
	// decrease — can't underflow into a garbage percentage; then clamp.
	var cpuPct float64
	if dTotal := float64(b.total) - float64(a.total); dTotal > 0 {
		dIdle := float64(b.idle) - float64(a.idle)
		cpuPct = (dTotal - dIdle) / dTotal * 100
		cpuPct = max(0, min(100, cpuPct))
	}

	var used uint64
	if memTotal > memAvail {
		used = memTotal - memAvail
	}

	u := Usage{
		Cores:          cores,
		MemTotalKB:     memTotal,
		MemAvailableKB: memAvail,
		MemUsedKB:      used,
		CPUPercent:     cpuPct,
	}
	// Trailing best-effort metrics: a missing/flaky df or uptime must not fail
	// the call, so parse errors leave the field at its zero value.
	if len(lines) > 5 {
		u.UptimeSeconds, _ = strconv.ParseFloat(lines[5], 64)
	}
	if len(lines) > 6 {
		u.DiskTotalGB, u.DiskUsedGB, _ = parseDiskLine(lines[6])
	}
	return u, nil
}

// parseDiskLine parses the "df -BG /" summary that awk reduces to "<total>G
// <used>G" (e.g. "20G 5G"), returning gigabytes.
func parseDiskLine(s string) (total, used float64, err error) {
	f := strings.Fields(s)
	if len(f) < 2 {
		return 0, 0, fmt.Errorf("stats: malformed df line: %q", s)
	}
	if total, err = parseGB(f[0]); err != nil {
		return 0, 0, err
	}
	if used, err = parseGB(f[1]); err != nil {
		return 0, 0, err
	}
	return total, used, nil
}

// parseGB parses a df -BG field like "20G" into gigabytes.
func parseGB(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSuffix(s, "G"), 64)
	if err != nil {
		return 0, fmt.Errorf("stats: parsing disk size %q: %w", s, err)
	}
	return v, nil
}

// cpuSample holds cumulative jiffies from one /proc/stat aggregate line.
type cpuSample struct {
	total uint64 // user..steal (guest/guest_nice excluded — they're already in user/nice)
	idle  uint64 // idle + iowait
}

// parseCPULine parses the aggregate "cpu  user nice system idle iowait irq
// softirq steal guest guest_nice" line. The trailing guest and guest_nice
// fields are deliberately excluded from total: the kernel already accounts that
// time in user and nice, so summing them again would inflate the denominator.
func parseCPULine(s string) (cpuSample, error) {
	f := strings.Fields(s)
	if len(f) < 5 || f[0] != "cpu" {
		return cpuSample{}, fmt.Errorf("stats: malformed /proc/stat line: %q", s)
	}
	var total, idle uint64
	for i, v := range f[1:] {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return cpuSample{}, fmt.Errorf("stats: parsing /proc/stat field %q: %w", v, err)
		}
		if i < 8 { // user, nice, system, idle, iowait, irq, softirq, steal
			total += n
		}
		if i == 3 || i == 4 { // idle, iowait
			idle += n
		}
	}
	return cpuSample{total: total, idle: idle}, nil
}

// nonEmptyLines splits raw into trimmed, non-empty lines.
func nonEmptyLines(raw []byte) []string {
	var out []string
	for l := range strings.SplitSeq(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

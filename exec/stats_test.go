package exec

import (
	"context"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

func TestStats_RoundTrip(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "statsbox")
	u, err := Stats(context.Background(), sb)
	require.NoError(t, err)
	require.Equal(t, 8, u.Cores)
	require.Equal(t, uint64(16384000), u.MemTotalKB)
	require.Equal(t, uint64(8192000), u.MemUsedKB)
	require.InDelta(t, 27.27, u.CPUPercent, 0.01)
	require.InDelta(t, 12345.67, u.UptimeSeconds, 0.01)
	require.Equal(t, 20.0, u.DiskTotalGB)
	require.Equal(t, 5.0, u.DiskUsedGB)
}

func TestStats_NonZeroExitIsError(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "statsfail")
	_, err := Stats(context.Background(), sb)
	require.Error(t, err)
	require.ErrorContains(t, err, "exited 1")
}

// probe builds the full 7-line probe output in script order: cores, MemTotal,
// MemAvailable, two /proc/stat samples, uptime, and "df total/used".
func probe(cores, memTotal, memAvail, cpuA, cpuB, uptime, disk string) []byte {
	return []byte(strings.Join([]string{cores, memTotal, memAvail, cpuA, cpuB, uptime, disk}, "\n") + "\n")
}

func TestParseStats_HappyPath(t *testing.T) {
	// A: user=100 nice=0 system=50 idle=1000 → total=1150 idle=1000
	// B: user=110 nice=0 system=55 idle=1040 → total=1205 idle=1040
	// (ΔTotal-ΔIdle)/ΔTotal = (55-40)/55 = 27.27%
	raw := probe("8", "16384000", "8192000",
		"cpu  100 0 50 1000 0 0 0 0 0 0",
		"cpu  110 0 55 1040 0 0 0 0 0 0",
		"12345.67", "20G 5G")

	st, err := parseStats(raw)
	require.NoError(t, err)
	require.Equal(t, 8, st.Cores)
	require.Equal(t, uint64(16384000), st.MemTotalKB)
	require.Equal(t, uint64(8192000), st.MemAvailableKB)
	require.Equal(t, uint64(8192000), st.MemUsedKB) // total - available
	require.InDelta(t, 27.27, st.CPUPercent, 0.01)
	require.InDelta(t, 12345.67, st.UptimeSeconds, 0.01)
	require.Equal(t, 20.0, st.DiskTotalGB) // "20G" → 20
	require.Equal(t, 5.0, st.DiskUsedGB)   // "5G"  → 5
}

func TestParseStats_ZeroCPUDelta(t *testing.T) {
	// Identical samples (no time elapsed) must yield 0%, not NaN/Inf.
	raw := probe("4", "1000", "400",
		"cpu  100 0 50 1000 0 0 0 0 0 0",
		"cpu  100 0 50 1000 0 0 0 0 0 0",
		"100.5", "10G 2G")

	st, err := parseStats(raw)
	require.NoError(t, err)
	require.Equal(t, 0.0, st.CPUPercent)
}

func TestParseStats_ClampsWhenBusyGoesBackwards(t *testing.T) {
	// idle climbs faster than total (iowait is non-monotonic in the kernel), so
	// the busy delta is negative. Must clamp to 0, not underflow to ~1e19.
	// A: total=1000 idle=500 ; B: total=1040 idle=580 → ΔTotal=40, ΔIdle=80.
	raw := probe("4", "1000", "400",
		"cpu  300 0 200 500 0 0 0 0 0 0",
		"cpu  300 0 160 580 0 0 0 0 0 0",
		"100.5", "10G 2G")

	st, err := parseStats(raw)
	require.NoError(t, err)
	require.Equal(t, 0.0, st.CPUPercent)
}

func TestParseStats_GuestNotDoubleCounted(t *testing.T) {
	// guest/guest_nice (last two fields) are already inside user/nice; they must
	// not inflate total. Here busy=user grows by 10 jiffies out of a 20 jiffy
	// window → 50%, regardless of the (changing) guest columns.
	raw := probe("1", "1000", "500",
		"cpu  100 0 0 1000 0 0 0 0 50 0",
		"cpu  110 0 0 1010 0 0 0 0 60 0",
		"100.5", "10G 2G")

	st, err := parseStats(raw)
	require.NoError(t, err)
	require.InDelta(t, 50.0, st.CPUPercent, 0.01)
}

func TestParseStats_TruncatedOutput(t *testing.T) {
	// Fewer lines than the core metrics need (nproc/mem/two cpu samples).
	_, err := parseStats([]byte("4\n1000\n400\ncpu  1 2 3 4 5\n"))
	require.Error(t, err)
}

func TestParseStats_MalformedCPULine(t *testing.T) {
	raw := probe("4", "1000", "400",
		"cpu  1 2 3 4 5",
		"not-a-cpu-line",
		"100.5", "10G 2G")
	_, err := parseStats(raw)
	require.ErrorContains(t, err, "/proc/stat")
}

func TestParseStats_CoreMetricsSurviveMissingDiskAndUptime(t *testing.T) {
	// Only the five core lines (no uptime, no df) — e.g. a busybox df that errored.
	// CPU/memory must still parse; disk/uptime fall back to 0 with no error.
	raw := []byte("8\n16384000\n8192000\n" +
		"cpu  100 0 50 1000 0 0 0 0 0 0\n" +
		"cpu  110 0 55 1040 0 0 0 0 0 0\n")

	st, err := parseStats(raw)
	require.NoError(t, err)
	require.Equal(t, 8, st.Cores)
	require.Equal(t, uint64(8192000), st.MemUsedKB)
	require.InDelta(t, 27.27, st.CPUPercent, 0.01)
	require.Equal(t, 0.0, st.UptimeSeconds)
	require.Equal(t, 0.0, st.DiskTotalGB)
	require.Equal(t, 0.0, st.DiskUsedGB)
}

func TestParseStats_MalformedDiskIsBestEffort(t *testing.T) {
	// A present-but-garbage df line zeroes the disk fields without failing.
	raw := probe("8", "16384000", "8192000",
		"cpu  100 0 50 1000 0 0 0 0 0 0",
		"cpu  110 0 55 1040 0 0 0 0 0 0",
		"12345.67", "nonsense")

	st, err := parseStats(raw)
	require.NoError(t, err)
	require.Equal(t, 8, st.Cores)
	require.InDelta(t, 12345.67, st.UptimeSeconds, 0.01)
	require.Equal(t, 0.0, st.DiskTotalGB)
	require.Equal(t, 0.0, st.DiskUsedGB)
}

package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Captured from GET /sandbox/{name} at sbx v0.51.0 (daemon api 0.49.0), for a
// sandbox with a workspace and a read-only additional workspace. Trimmed to
// the keys typed since then.
const sandboxInfoV051 = `{
  "id": "x", "name": "p6-ro", "status": "running", "workspace": "/tmp/p6ws",
  "working_dir": "",
  "effective_working_dir": "/tmp/p6ws",
  "host_mounts": [
    {"container_target": "/tmp/p6ws", "host_path": "/tmp/p6ws", "read_only": false},
    {"container_target": "/tmp/p6ro", "host_path": "/tmp/p6ro", "read_only": true}
  ],
  "runtime_mounts": [],
  "last_used_at": "2026-11-14T06:22:55.190610476Z"
}`

func TestSandboxInfo_DecodesV051Keys(t *testing.T) {
	var info SandboxInfo
	require.NoError(t, json.Unmarshal([]byte(sandboxInfoV051), &info))

	require.NotNil(t, info.WorkingDir)
	require.Equal(t, "", *info.WorkingDir)
	require.NotNil(t, info.EffectiveWorkingDir)
	require.Equal(t, "/tmp/p6ws", *info.EffectiveWorkingDir)
	require.NotNil(t, info.HostMounts)
	require.Equal(t, []HostMount{
		{ContainerTarget: "/tmp/p6ws", HostPath: "/tmp/p6ws", ReadOnly: false},
		{ContainerTarget: "/tmp/p6ro", HostPath: "/tmp/p6ro", ReadOnly: true},
	}, *info.HostMounts)
	require.NotNil(t, info.RuntimeMounts)
	require.Empty(t, *info.RuntimeMounts)
	require.NotNil(t, info.LastUsedAt)
	require.Equal(t, time.Date(2026, 11, 14, 6, 22, 55, 190610476, time.UTC), *info.LastUsedAt)
}

// A pre-v0.43.0 daemon sends none of them; they stay nil.
func TestSandboxInfo_OldDaemonLeavesNewKeysNil(t *testing.T) {
	var info SandboxInfo
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x","name":"n","status":"running","workspace":"/w"}`), &info))
	require.Nil(t, info.WorkingDir)
	require.Nil(t, info.EffectiveWorkingDir)
	require.Nil(t, info.HostMounts)
	require.Nil(t, info.RuntimeMounts)
	require.Nil(t, info.LastUsedAt)
}

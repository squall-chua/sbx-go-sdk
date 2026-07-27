package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func clientWithRecordingSbx(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestCopyTo(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "s1")

	require.NoError(t, sb.CopyTo(context.Background(), "/local/a.txt", "/home/user/a.txt", WithFollowSymlinks()))

	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "cp -L /local/a.txt s1:/home/user/a.txt")
}

// tarOf builds a one-entry tar for the fake /files handler.
func tarOf(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write([]byte(body))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	return buf.Bytes()
}

func TestCopyFrom_FileToNewDestination(t *testing.T) {
	var gotPath, gotFollow string
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1":
			w.Write([]byte(`{"id":"i1","name":"s1","status":"running","workspace":"/ws"}`))
		case "/sandbox/s1/files":
			gotPath = r.URL.Query().Get("path")
			gotFollow = r.URL.Query().Get("follow")
			w.Header().Set("Content-Type", "application/x-tar")
			w.Write(tarOf(t, "top.txt", "hello"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	sb := NewForTest(c, "s1")

	dst := filepath.Join(t.TempDir(), "got.txt")
	require.NoError(t, sb.CopyFrom(context.Background(), "/ws/top.txt", dst))

	require.Equal(t, "/ws/top.txt", gotPath)
	require.Empty(t, gotFollow, "follow must be absent unless WithFollowSymlinks is set")

	b, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))
}

func TestCopyFrom_FileIntoExistingDirectory(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1":
			w.Write([]byte(`{"id":"i1","name":"s1","status":"running","workspace":"/ws"}`))
		case "/sandbox/s1/files":
			w.Write(tarOf(t, "top.txt", "hello"))
		}
	}))
	sb := NewForTest(c, "s1")

	dst := t.TempDir() // already exists as a directory
	require.NoError(t, sb.CopyFrom(context.Background(), "/ws/top.txt", dst))

	b, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))
}

func TestCopyFrom_WithFollowSymlinksSetsFollowParam(t *testing.T) {
	var gotFollow string
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1":
			w.Write([]byte(`{"id":"i1","name":"s1","status":"running","workspace":"/ws"}`))
		case "/sandbox/s1/files":
			gotFollow = r.URL.Query().Get("follow")
			w.Write(tarOf(t, "link.txt", "resolved"))
		}
	}))
	sb := NewForTest(c, "s1")

	dst := filepath.Join(t.TempDir(), "out.txt")
	require.NoError(t, sb.CopyFrom(context.Background(), "/ws/link.txt", dst, WithFollowSymlinks()))
	require.Equal(t, "true", gotFollow)
}

func TestCopyFrom_AutoStartsStoppedSandbox(t *testing.T) {
	started := false
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1":
			status := "stopped"
			if started {
				status = "running"
			}
			w.Write([]byte(`{"id":"i1","name":"s1","status":"` + status + `","workspace":"/ws"}`))
		case "/sandbox/s1/start":
			started = true
			w.WriteHeader(http.StatusOK)
		case "/sandbox/s1/files":
			require.True(t, started, "files must not be fetched before the sandbox is started")
			w.Write(tarOf(t, "top.txt", "hello"))
		}
	}))
	sb := NewForTest(c, "s1")

	dst := filepath.Join(t.TempDir(), "got.txt")
	require.NoError(t, sb.CopyFrom(context.Background(), "/ws/top.txt", dst))
	require.True(t, started, "CopyFrom must auto-start a stopped sandbox, matching sbx cp")
}

func TestCopyFrom_MissingPathReturnsError(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1":
			w.Write([]byte(`{"id":"i1","name":"s1","status":"running","workspace":"/ws"}`))
		case "/sandbox/s1/files":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message":"path \"/ws/nope\" not found in sandbox \"s1\""}`))
		}
	}))
	sb := NewForTest(c, "s1")

	err := sb.CopyFrom(context.Background(), "/ws/nope", filepath.Join(t.TempDir(), "x"))
	require.Error(t, err)
}

func TestCopyFrom_FailedExtractionLeavesNoPartialDestination(t *testing.T) {
	// An escaping symlink aborts extraction; the destination must not appear.
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox/s1":
			w.Write([]byte(`{"id":"i1","name":"s1","status":"running","workspace":"/ws"}`))
		case "/sandbox/s1/files":
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			require.NoError(t, tw.WriteHeader(&tar.Header{
				Name: "evil/esc", Typeflag: tar.TypeSymlink, Linkname: "../../../../etc/hostname",
			}))
			require.NoError(t, tw.Close())
			w.Write(buf.Bytes())
		}
	}))
	sb := NewForTest(c, "s1")

	dst := filepath.Join(t.TempDir(), "evil")
	err := sb.CopyFrom(context.Background(), "/ws/evil", dst)
	require.Error(t, err)
	require.NoFileExists(t, dst)
	require.NoDirExists(t, dst)
}

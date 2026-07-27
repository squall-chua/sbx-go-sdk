package untar

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// entry describes one tar member for the test builder.
type entry struct {
	name     string
	mode     int64
	body     string
	typeflag byte
	linkname string
}

func buildTar(t *testing.T, entries ...entry) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		tf := e.typeflag
		if tf == 0 {
			tf = tar.TypeReg
		}
		h := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Size:     int64(len(e.body)),
			Typeflag: tf,
			Linkname: e.linkname,
			Uname:    "root",
			Gname:    "root",
		}
		if tf == tar.TypeDir || tf == tar.TypeSymlink {
			h.Size = 0
		}
		require.NoError(t, tw.WriteHeader(h))
		if tf == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	return &buf
}

func TestExtract_FileWithMode(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t, entry{name: "top.txt", mode: 0o644, body: "hello"}), dst)
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))

	fi, err := os.Stat(filepath.Join(dst, "top.txt"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), fi.Mode().Perm())
}

func TestExtract_PreservesExecutableMode(t *testing.T) {
	dst := t.TempDir()
	require.NoError(t, Extract(buildTar(t, entry{name: "run.sh", mode: 0o755, body: "#!/bin/sh\n"}), dst))
	fi, err := os.Stat(filepath.Join(dst, "run.sh"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), fi.Mode().Perm())
}

func TestExtract_PreservesRestrictedMode(t *testing.T) {
	dst := t.TempDir()
	require.NoError(t, Extract(buildTar(t, entry{name: "secret.txt", mode: 0o640, body: "x"}), dst))
	fi, err := os.Stat(filepath.Join(dst, "secret.txt"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), fi.Mode().Perm())
}

func TestExtract_NestedTree(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "dir/", mode: 0o755, typeflag: tar.TypeDir},
		entry{name: "dir/mid.txt", mode: 0o644, body: "m"},
		entry{name: "dir/nested/", mode: 0o755, typeflag: tar.TypeDir},
		entry{name: "dir/nested/deep.txt", mode: 0o644, body: "d"},
	), dst)
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(dst, "dir", "nested", "deep.txt"))
	require.NoError(t, err)
	require.Equal(t, "d", string(b))
}

func TestExtract_FileBeforeItsParentDirEntry(t *testing.T) {
	// The daemon is not required to emit parent directories first.
	dst := t.TempDir()
	err := Extract(buildTar(t, entry{name: "a/b/c.txt", mode: 0o644, body: "c"}), dst)
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dst, "a", "b", "c.txt"))
	require.NoError(t, err)
	require.Equal(t, "c", string(b))
}

func TestExtract_IgnoresTarOwnership(t *testing.T) {
	// Entries are root/root but extraction must produce files owned by the
	// calling user, matching `sbx cp`. Simply not erroring proves we never
	// attempted a chown, which would fail as an unprivileged user.
	dst := t.TempDir()
	require.NoError(t, Extract(buildTar(t, entry{name: "o.txt", mode: 0o644, body: "o"}), dst))
	require.FileExists(t, filepath.Join(dst, "o.txt"))
}

func TestExtract_RelativeSymlinkInsideRootIsKept(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "real.txt", mode: 0o644, body: "r"},
		entry{name: "link.txt", typeflag: tar.TypeSymlink, linkname: "real.txt"},
	), dst)
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dst, "link.txt"))
	require.NoError(t, err)
	require.Equal(t, "real.txt", target)
}

func TestExtract_AbsoluteSymlinkIsAllowed(t *testing.T) {
	// Matches `sbx cp`, which creates "-> /etc/passwd" links and exits 0.
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "abs.txt", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
	), dst)
	require.NoError(t, err)

	target, err := os.Readlink(filepath.Join(dst, "abs.txt"))
	require.NoError(t, err)
	require.Equal(t, "/etc/passwd", target)
}

func TestExtract_EscapingRelativeSymlinkIsRejected(t *testing.T) {
	// os.Root permits CREATING an escaping symlink, so this must be caught
	// explicitly. `sbx cp` exits 1 on exactly this input.
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "esc.txt", typeflag: tar.TypeSymlink, linkname: "../../../../etc/hostname"},
	), dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes destination")
	require.NoFileExists(t, filepath.Join(dst, "esc.txt"))
}

func TestExtract_EscapingSymlinkViaSubdirIsRejected(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "sub/esc.txt", typeflag: tar.TypeSymlink, linkname: "../../outside"},
	), dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "escapes destination")
}

func TestExtract_SymlinkOneLevelUpInsideRootIsKept(t *testing.T) {
	// "sub/back.txt -> ../real.txt" resolves to "real.txt", still inside.
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "real.txt", mode: 0o644, body: "r"},
		entry{name: "sub/back.txt", typeflag: tar.TypeSymlink, linkname: "../real.txt"},
	), dst)
	require.NoError(t, err)
	target, err := os.Readlink(filepath.Join(dst, "sub", "back.txt"))
	require.NoError(t, err)
	require.Equal(t, "../real.txt", target)
}

func TestExtract_PathTraversalEntryIsRejected(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t, entry{name: "../escape.txt", mode: 0o644, body: "x"}), dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsafe tar entry")
}

func TestExtract_AbsoluteEntryPathIsRejected(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t, entry{name: "/etc/passwd", mode: 0o644, body: "x"}), dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsafe tar entry")
}

func TestExtract_UnsupportedEntryTypeIsRejected(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t, entry{name: "fifo", typeflag: tar.TypeFifo}), dst)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported tar entry")
}

func TestExtract_CreatesDestDirIfMissing(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "does", "not", "exist")
	require.NoError(t, Extract(buildTar(t, entry{name: "f.txt", mode: 0o644, body: "f"}), dst))
	require.FileExists(t, filepath.Join(dst, "f.txt"))
}

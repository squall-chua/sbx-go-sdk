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

func TestExtract_PreservesDirMode(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t, entry{name: "dir/", mode: 0o700, typeflag: tar.TypeDir}), dst)
	require.NoError(t, err)

	fi, err := os.Stat(filepath.Join(dst, "dir"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
}

func TestExtract_DirModeAppliedWhenDirEntryFollowsItsContents(t *testing.T) {
	// mkParents creates "dir" at 0o755 when "dir/file.txt" arrives first;
	// the dir's own entry (0o700) must still win when it arrives later.
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "dir/file.txt", mode: 0o644, body: "f"},
		entry{name: "dir/", mode: 0o700, typeflag: tar.TypeDir},
	), dst)
	require.NoError(t, err)

	dfi, err := os.Stat(filepath.Join(dst, "dir"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dfi.Mode().Perm())

	ffi, err := os.Stat(filepath.Join(dst, "dir", "file.txt"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o644), ffi.Mode().Perm())
}

func TestExtract_RestrictiveDirModeDoesNotBlockLaterEntries(t *testing.T) {
	// A "dir/" entry at 0o500 arriving between two of its own file entries
	// must not lock the directory before the later entry is written.
	dst := t.TempDir()
	dirPath := filepath.Join(dst, "dir")
	t.Cleanup(func() { _ = os.Chmod(dirPath, 0o755) })

	err := Extract(buildTar(t,
		entry{name: "dir/file1.txt", mode: 0o644, body: "one"},
		entry{name: "dir/", mode: 0o500, typeflag: tar.TypeDir},
		entry{name: "dir/file2.txt", mode: 0o644, body: "two"},
	), dst)
	require.NoError(t, err)

	b1, err := os.ReadFile(filepath.Join(dirPath, "file1.txt"))
	require.NoError(t, err)
	require.Equal(t, "one", string(b1))

	b2, err := os.ReadFile(filepath.Join(dirPath, "file2.txt"))
	require.NoError(t, err)
	require.Equal(t, "two", string(b2))

	fi, err := os.Stat(dirPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o500), fi.Mode().Perm())
}

func TestExtract_RestrictiveDirModeAppliedBeforeContents(t *testing.T) {
	// A "dir/" entry at 0o500 arriving before its own contents must not
	// block writing those contents either.
	dst := t.TempDir()
	dirPath := filepath.Join(dst, "dir")
	t.Cleanup(func() { _ = os.Chmod(dirPath, 0o755) })

	err := Extract(buildTar(t,
		entry{name: "dir/", mode: 0o500, typeflag: tar.TypeDir},
		entry{name: "dir/file.txt", mode: 0o644, body: "f"},
	), dst)
	require.NoError(t, err)

	b, err := os.ReadFile(filepath.Join(dirPath, "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "f", string(b))

	fi, err := os.Stat(dirPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o500), fi.Mode().Perm())
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

func TestExtract_SkipsPaxGlobalHeader(t *testing.T) {
	// `git archive` and GNU tar in pax mode emit a pax_global_header entry.
	// Go's tar.Reader surfaces it to the caller instead of consuming it, so a
	// naive switch aborts the whole extraction before writing anything.
	//
	// tar.Writer requires a TypeXGlobalHeader header to carry only Name and
	// PAXRecords (everything else must be the zero value), so this is built
	// directly rather than via buildTar/entry, which also sets Uname/Gname.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Typeflag:   tar.TypeXGlobalHeader,
		Name:       "pax_global_header",
		PAXRecords: map[string]string{"comment": "example"},
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "top.txt", Mode: 0o644, Size: 5, Typeflag: tar.TypeReg,
		Uname: "root", Gname: "root",
	}))
	_, err := tw.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	dst := t.TempDir()
	require.NoError(t, Extract(&buf, dst))

	b, err := os.ReadFile(filepath.Join(dst, "top.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))
}

func TestExtract_RegularFileReplacesExistingSymlink(t *testing.T) {
	dst := t.TempDir()
	err := Extract(buildTar(t,
		entry{name: "real.txt", mode: 0o644, body: "r"},
		entry{name: "link.txt", typeflag: tar.TypeSymlink, linkname: "real.txt"},
		entry{name: "link.txt", mode: 0o644, body: "replaced"},
	), dst)
	require.NoError(t, err)

	fi, err := os.Lstat(filepath.Join(dst, "link.txt"))
	require.NoError(t, err)
	require.Zero(t, fi.Mode()&os.ModeSymlink, "link.txt must be a regular file, not a symlink")

	b, err := os.ReadFile(filepath.Join(dst, "link.txt"))
	require.NoError(t, err)
	require.Equal(t, "replaced", string(b))

	real, err := os.ReadFile(filepath.Join(dst, "real.txt"))
	require.NoError(t, err)
	require.Equal(t, "r", string(real), "real.txt must be untouched")
}

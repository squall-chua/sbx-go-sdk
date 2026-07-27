// Package untar extracts a tar stream into a destination directory, confined so
// that no entry can write outside it.
//
// The tar streams this handles come from `GET /sandbox/{name}/files`, whose
// contents are authored inside a sandbox by an AI agent. They are untrusted
// input.
package untar

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
)

// Extract writes the tar stream in r into destDir, creating destDir if needed.
//
// Behaviour matches `sbx cp` deliberately:
//   - Ownership recorded in the tar is ignored. Entries arrive as root/root and
//     extracted files belong to the calling user.
//   - Permission bits are preserved.
//   - A relative symlink target that resolves outside destDir is an error; an
//     absolute target is allowed.
//
// Hardlink entries (tar.TypeLink) are rejected rather than recreated as
// hardlinks or copies — a known divergence from `sbx cp`, which does handle
// them.
func Extract(r io.Reader, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destDir)
	if err != nil {
		return err
	}
	defer root.Close()

	dirModes := map[string]os.FileMode{}

	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return applyDirModes(root, dirModes)
		}
		if err != nil {
			return err
		}
		name, err := safeName(h.Name)
		if err != nil {
			return err
		}
		if name == "" {
			continue // the archive root itself
		}
		mode := h.FileInfo().Mode().Perm()

		switch h.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0o755); err != nil {
				return err
			}
			dirModes[name] = mode
		case tar.TypeReg:
			if err := mkParents(root, name); err != nil {
				return err
			}
			if err := writeFile(root, tr, name, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// os.Root blocks traversal THROUGH an escaping symlink but still
			// permits creating one, so the target is checked explicitly.
			if err := checkSymlinkTarget(name, h.Linkname); err != nil {
				return err
			}
			if err := mkParents(root, name); err != nil {
				return err
			}
			_ = root.Remove(name)
			if err := root.Symlink(h.Linkname, name); err != nil {
				return fmt.Errorf("symlink %s -> %s: %w", name, h.Linkname, err)
			}
		case tar.TypeXGlobalHeader:
			// Go's tar.Reader surfaces the pax global header rather than consuming
			// it, unlike TypeXHeader. It carries no file data — skip it.
			continue
		default:
			return fmt.Errorf("unsupported tar entry type %q for %s", h.Typeflag, h.Name)
		}
	}
}

// safeName cleans a tar entry name and rejects absolute or escaping paths.
// It returns "" for the archive root ("." or "/").
func safeName(raw string) (string, error) {
	n := path.Clean(strings.TrimPrefix(raw, "./"))
	if n == "." || n == "/" || n == "" {
		return "", nil
	}
	if path.IsAbs(n) || n == ".." || strings.HasPrefix(n, "../") {
		return "", fmt.Errorf("refusing unsafe tar entry %q", raw)
	}
	return n, nil
}

// checkSymlinkTarget rejects a relative target that resolves outside the root.
// Absolute targets are permitted, matching `sbx cp`.
func checkSymlinkTarget(entry, target string) error {
	if target == "" {
		return fmt.Errorf("invalid symlink %q: empty target", entry)
	}
	if path.IsAbs(target) {
		return nil
	}
	resolved := path.Clean(path.Join(path.Dir(entry), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("invalid symlink %q -> %q: escapes destination", entry, target)
	}
	return nil
}

// applyDirModes restores recorded directory modes after the whole stream is
// extracted. Applying them inline would lock a directory before later entries
// are written into it: a "dir/" entry at 0o500 sitting between two of its own
// file entries would abort the extraction.
//
// Deepest first, so a child is reached before its parent loses the execute bit.
func applyDirModes(root *os.Root, modes map[string]os.FileMode) error {
	names := make([]string, 0, len(modes))
	for n := range modes {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		if err := root.Chmod(n, modes[n]); err != nil {
			return err
		}
	}
	return nil
}

func mkParents(root *os.Root, name string) error {
	if d := path.Dir(name); d != "." && d != "/" {
		return root.MkdirAll(d, 0o755)
	}
	return nil
}

func writeFile(root *os.Root, src io.Reader, name string, mode os.FileMode) error {
	// A pre-existing symlink at name must be replaced, not written through —
	// mirrors the TypeSymlink branch above.
	_ = root.Remove(name)
	f, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// O_CREATE honours the process umask, so restore the recorded mode.
	return root.Chmod(name, mode)
}

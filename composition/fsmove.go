package composition

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// This file holds the cross-filesystem move helpers the trash
// subsystem uses to relocate files. The helpers live in their
// own file because they have nothing to do with the trash
// model itself. They are general filesystem plumbing that
// belongs together and apart from the higher-level trash
// logic in trash.go.

// moveFileOrDir moves a path from src to dst. It tries
// os.Rename first because that is atomic and cheap when both
// paths sit on the same filesystem. When rename fails with a
// cross-device error (EXDEV on Linux and macOS), the function
// falls back to copying the data over and removing the
// source.
//
// The copy+remove fallback is not atomic. If the process
// crashes mid-copy, the user can be left with partial data on
// either side. We accept that risk because the alternative is
// to refuse cross-device moves entirely, which would break
// for any user whose home directory and trash directory
// happen to sit on different volumes.
func moveFileOrDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Rename failed. Determine whether the source is a file,
	// a directory, or something we cannot copy.
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := copyDir(src, dst); err != nil {
			return err
		}
		return os.RemoveAll(src)
	}
	if err := copyFile(src, dst, info.Mode()); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile copies the contents of src to dst, preserving the
// file mode. The destination is created with the same
// permissions as the source so a restored file matches what
// the user had before trashing.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// copyDir recursively copies a directory tree. The function
// is the fallback inside moveFileOrDir when os.Rename cannot
// cross a filesystem boundary. Symbolic links inside the
// tree are preserved as links, not followed, because
// following could read or write outside the source tree.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode())
		}
	})
}

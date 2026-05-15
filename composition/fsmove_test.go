package composition

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// TestMoveFileOrDir_movesAFileWithSameFilesystemRename covers
// the happy path. When src and dst sit on the same volume,
// os.Rename succeeds and we never enter the copy fallback.
// The test temp dir is one volume, so both arguments land
// there and rename wins.
func TestMoveFileOrDir_movesAFileWithSameFilesystemRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := moveFileOrDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("source should be gone, stat err = %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("dest contents = %q, want hello", got)
	}
}

// TestMoveFileOrDir_movesADirectoryTree covers the directory
// path. The function handles both files and directories
// because rename works for either, so the same code path
// applies to a directory tree.
func TestMoveFileOrDir_movesADirectoryTree(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source-tree")
	if err := os.MkdirAll(filepath.Join(src, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "inner", "x.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "moved-tree")
	if err := moveFileOrDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("source tree should be gone, stat err = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "inner", "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "nested" {
		t.Errorf("nested file contents = %q, want nested", got)
	}
}

// TestCopyFile_preservesModeAndContents pins the copyFile
// helper. It is the cross-volume fallback inside
// moveFileOrDir, so when rename works we never see it run.
// Calling it directly is the only way to confirm the mode
// gets preserved and the contents survive a full byte
// round-trip.
func TestCopyFile_preservesModeAndContents(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")

	want := []byte{0x00, 0x01, 0x02, 0xFE, 0xFF}
	if err := os.WriteFile(src, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("dest bytes differ from source bytes")
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0o600", info.Mode().Perm())
	}
}

// TestCopyDir_recursesAndPreservesStructure pins the
// directory-copy helper. It is the other half of the
// fallback inside moveFileOrDir and only runs when a rename
// cannot cross a volume. Calling it directly confirms a
// nested structure survives the copy intact.
func TestCopyDir_recursesAndPreservesStructure(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(filepath.Join(src, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a", "b", "leaf.txt"), []byte("deep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "dst")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "a", "b", "leaf.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "deep" {
		t.Errorf("nested file lost: got %q", got)
	}
}

// TestCopyDir_preservesSymlinks confirms the symlink branch
// of copyDir. The helper deliberately preserves symlinks
// instead of following them, because following could read
// or write outside the source tree. The test plants a
// symlink, copies the tree, and checks that the destination
// also has a symlink (and not a copy of the target).
func TestCopyDir_preservesSymlinks(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "elsewhere.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "dst")
	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(dst, "link"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("copied entry should still be a symlink")
	}
}

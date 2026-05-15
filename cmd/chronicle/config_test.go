package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureConfigFileExists_createsParentAndFile is the
// happy path. We hand the helper a path inside a fresh temp
// directory plus a not-yet-created subdirectory. After the
// call, both the directory and the file should exist with
// readable permissions.
func TestEnsureConfigFileExists_createsParentAndFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "subdir", "config.toml")

	if err := ensureConfigFileExists(target); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("config file should exist after ensure: %v", err)
	}
	if info.IsDir() {
		t.Errorf("expected a regular file, got a directory")
	}
}

// TestEnsureConfigFileExists_isIdempotent confirms running
// the helper twice in a row is safe. The second call must
// not truncate or otherwise damage an existing file.
func TestEnsureConfigFileExists_isIdempotent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "config.toml")

	if err := ensureConfigFileExists(target); err != nil {
		t.Fatal(err)
	}
	const sentinel = "trash.retention_days = 7\n"
	if err := os.WriteFile(target, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureConfigFileExists(target); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Errorf("ensureConfigFileExists clobbered an existing file: got %q, want %q", got, sentinel)
	}
}

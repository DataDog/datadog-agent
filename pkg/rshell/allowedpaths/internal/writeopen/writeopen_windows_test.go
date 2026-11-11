// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// These are Go tests by necessity: writeopen is an internal package and the
// YAML scenario framework cannot reach it directly, so AGENTS.md's "prefer
// scenario tests" rule does not apply here.
//
// The Windows implementation cannot share writeopen_unix_test.go: it ignores
// the *os.File rootFile argument entirely (OpenRoot returns (nil, nil) here)
// and drives everything off *os.Root plus NtCreateFile, so the Unix tests'
// fixtures and their errno expectations (ELOOP, EISDIR, ENOTDIR) do not
// apply. Symlink coverage is also deliberately absent: creating a symlink on
// Windows requires SeCreateSymbolicLinkPrivilege or developer mode, which CI
// runners generally do not have, so a symlink test would be skipped in
// practice rather than run. Symlink/reparse-point rejection on Windows is
// enforced by os.Root's O_NOFOLLOW_ANY walk and by FILE_OPEN_REPARSE_POINT,
// and is covered transitively by the allowedpaths package tests.

package writeopen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newRoot(t *testing.T) (string, *os.Root, *os.File) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = root.Close() })
	rootFile, err := OpenRoot(root)
	if err != nil {
		t.Fatalf("writeopen.OpenRoot: %v", err)
	}
	t.Cleanup(func() { CloseRoot(rootFile) })
	return dir, root, rootFile
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

// TestOpenRootReturnsNilOnWindows pins the platform contract the Windows
// Unlink/OpenFile rely on: there is no directory handle to walk from, so
// OpenRoot yields (nil, nil) and CloseRoot is a no-op.
func TestOpenRootReturnsNilOnWindows(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	f, err := OpenRoot(root)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	if f != nil {
		t.Fatalf("OpenRoot = %v, want nil on Windows", f)
	}
	CloseRoot(f)
	CloseRoot(nil)
}

func TestOpenFileHappyPathNested(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "a", "b"))

	f, err := OpenFile(rootFile, root, "a/b/new.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString("hello"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "a", "b", "new.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(contents) != "hello" {
		t.Fatalf("contents = %q, want %q", contents, "hello")
	}
}

func TestOpenFileParentTraversalRejected(t *testing.T) {
	_, root, rootFile := newRoot(t)
	f, err := OpenFile(rootFile, root, "../escape.txt", os.O_WRONLY|os.O_CREATE, 0o600)
	if err == nil {
		_ = f.Close()
		t.Fatal("OpenFile returned a file for a traversal path")
	}
}

func TestOpenFileMissingIntermediateDirectory(t *testing.T) {
	_, root, rootFile := newRoot(t)
	f, err := OpenFile(rootFile, root, "nope/new.txt", os.O_WRONLY|os.O_CREATE, 0o600)
	if f != nil {
		_ = f.Close()
		t.Fatal("OpenFile returned a file under a missing directory")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestUnlinkRemovesRegularFile(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "a", "b"))
	target := filepath.Join(dir, "a", "b", "gone.txt")
	mustWriteFile(t, target, "bye")

	if err := Unlink(rootFile, root, "a/b/gone.txt"); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file still present: Lstat err = %v, want ErrNotExist", err)
	}
}

func TestUnlinkDirectoryRejected(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "empty"))
	mustMkdir(t, filepath.Join(dir, "full", "child"))

	for _, relPath := range []string{"empty", "full"} {
		t.Run(relPath, func(t *testing.T) {
			if err := Unlink(rootFile, root, relPath); !errors.Is(err, ErrIsDirectory) {
				t.Fatalf("Unlink(%q) err = %v, want ErrIsDirectory", relPath, err)
			}
			if _, statErr := os.Lstat(filepath.Join(dir, relPath)); statErr != nil {
				t.Fatalf("directory was removed: %v", statErr)
			}
		})
	}
}

func TestUnlinkDirSyntaxOnlyPaths(t *testing.T) {
	for _, relPath := range []string{"", ".", "./", ".."} {
		t.Run("path="+relPath, func(t *testing.T) {
			_, root, rootFile := newRoot(t)
			if err := Unlink(rootFile, root, relPath); !errors.Is(err, ErrIsDirectory) {
				t.Fatalf("Unlink(%q) err = %v, want ErrIsDirectory", relPath, err)
			}
		})
	}
}

func TestUnlinkParentTraversalRejected(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	outside := filepath.Join(filepath.Dir(dir), "outside.txt")
	mustWriteFile(t, outside, "keep")
	t.Cleanup(func() { _ = os.Remove(outside) })

	if err := Unlink(rootFile, root, "../outside.txt"); err == nil {
		t.Fatal("Unlink removed a file outside the root")
	}
	if _, err := os.Lstat(outside); err != nil {
		t.Fatalf("file outside the root was removed: %v", err)
	}
}

func TestUnlinkTrailingDirSyntax(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		wantErr error
	}{
		{name: "file with trailing slash", relPath: "file.txt/", wantErr: ErrNotDirectory},
		{name: "file with trailing dot", relPath: "file.txt/.", wantErr: ErrNotDirectory},
		{name: "real dir with trailing slash", relPath: "realdir/", wantErr: ErrIsDirectory},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, root, rootFile := newRoot(t)
			mustWriteFile(t, filepath.Join(dir, "file.txt"), "keep")
			mustMkdir(t, filepath.Join(dir, "realdir"))

			if err := Unlink(rootFile, root, tc.relPath); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Unlink(%q) err = %v, want %v", tc.relPath, err, tc.wantErr)
			}
			for _, name := range []string{"file.txt", "realdir"} {
				if _, statErr := os.Lstat(filepath.Join(dir, name)); statErr != nil {
					t.Errorf("%s was removed: %v", name, statErr)
				}
			}
		})
	}
}

func TestUnlinkMissingFile(t *testing.T) {
	_, root, rootFile := newRoot(t)
	if err := Unlink(rootFile, root, "nope.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestUnlinkMissingIntermediateDirectory(t *testing.T) {
	_, root, rootFile := newRoot(t)
	if err := Unlink(rootFile, root, "nope/file.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

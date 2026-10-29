// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

// These are Go tests by necessity: writeopen is an internal package and the
// YAML scenario framework cannot reach it directly, so AGENTS.md's "prefer
// scenario tests" rule does not apply here.
//
// The Unix and Windows implementations of OpenFile/Unlink share their
// exported signatures but not their semantics or their inputs: the Unix
// walker drives a raw openat(2)/unlinkat(2) descriptor walk off rootFile and
// ignores the *os.Root, while the Windows implementation ignores rootFile and
// delegates to os.Root plus NtCreateFile. They also reject different things
// (ELOOP vs. STATUS_*; symlink creation on Windows needs a privilege most CI
// runners lack). The two therefore get separate test files rather than a
// shared one; see writeopen_windows_test.go.
//
// Error assertions use errors.Is against sentinels and unix errnos, never
// message strings, since the strerror text differs between Linux and macOS.
//
// Four defensive branches in writeopen_unix.go are deliberately not covered
// because no input can reach them:
//
//   - the closeDir cleanup inside the ".." intermediate-component checks
//     (writeopen_unix.go:46-48 and :120-122). filepath.Clean collapses any
//     ".." that follows a real component, so a surviving ".." is always the
//     first component and closeDir is still false when it is seen.
//   - the closeDir cleanup in OpenFile's final-component check
//     (writeopen_unix.go:64-66), for the same reason: a base of "" only
//     arises from "/" and a base of ".." only from "..", neither of which
//     opens an intermediate directory first.
//   - the os.NewFile == nil guard (writeopen_unix.go:77-80), which cannot
//     trigger for a descriptor openat has just returned successfully.
//
// They are correct as written; leaving them in place costs nothing and
// protects the invariant if the cleaning logic ever changes.

package writeopen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// newRoot returns an opened *os.Root over a fresh temp dir plus the *os.File
// directory handle the Unix implementation walks from.
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

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%q, %q): %v", target, link, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// OpenFile
// ---------------------------------------------------------------------------

func TestOpenFileNilRootFile(t *testing.T) {
	_, root, _ := newRoot(t)
	f, err := OpenFile(nil, root, "file.txt", os.O_WRONLY|os.O_CREATE, 0o600)
	if f != nil {
		_ = f.Close()
		t.Fatal("OpenFile with nil rootFile returned a file")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want os.ErrPermission", err)
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("err = %T, want *os.PathError", err)
	}
	if pathErr.Op != "openat" || pathErr.Path != "file.txt" {
		t.Errorf("PathError = {Op:%q Path:%q}, want {openat file.txt}", pathErr.Op, pathErr.Path)
	}
}

func TestOpenFileCleansToDot(t *testing.T) {
	// Every relPath whose filepath.Clean is "." must be EISDIR: the root
	// directory itself is never a write target.
	for _, relPath := range []string{"", ".", "./", "./."} {
		t.Run("path="+relPath, func(t *testing.T) {
			_, root, rootFile := newRoot(t)
			f, err := OpenFile(rootFile, root, relPath, os.O_WRONLY|os.O_CREATE, 0o600)
			if f != nil {
				_ = f.Close()
				t.Fatal("OpenFile returned a file for the root directory")
			}
			if !errors.Is(err, unix.EISDIR) {
				t.Fatalf("err = %v, want EISDIR", err)
			}
		})
	}
}

func TestOpenFileFinalComponentIsDirSyntax(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
	}{
		// filepath.Clean("/") == "/", split yields a trailing "" base.
		{name: "empty base", relPath: "/"},
		// filepath.Clean("..") == "..", so the base really is "..".
		{name: "dotdot base", relPath: ".."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, root, rootFile := newRoot(t)
			f, err := OpenFile(rootFile, root, tc.relPath, os.O_WRONLY|os.O_CREATE, 0o600)
			if f != nil {
				_ = f.Close()
				t.Fatal("OpenFile returned a file")
			}
			if !errors.Is(err, unix.EISDIR) {
				t.Fatalf("OpenFile(%q) err = %v, want EISDIR", tc.relPath, err)
			}
		})
	}
}

func TestOpenFileParentTraversalRejected(t *testing.T) {
	// A ".." intermediate component must be refused outright rather than
	// walked, so a write target can never escape the root.
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "sub"))

	for _, relPath := range []string{"../escape.txt", "../sub/escape.txt", "../../escape.txt"} {
		t.Run("path="+relPath, func(t *testing.T) {
			f, err := OpenFile(rootFile, root, relPath, os.O_WRONLY|os.O_CREATE, 0o600)
			if f != nil {
				_ = f.Close()
				t.Fatal("OpenFile returned a file for a traversal path")
			}
			if !errors.Is(err, os.ErrPermission) {
				t.Fatalf("OpenFile(%q) err = %v, want os.ErrPermission", relPath, err)
			}
		})
	}
	// Nothing escaped.
	if _, err := os.Lstat(filepath.Join(filepath.Dir(dir), "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escape.txt outside the root: Lstat err = %v, want ErrNotExist", err)
	}
}

// TestOpenFileNoFDLeakOnRejection is a cheap regression guard on the closeDir
// bookkeeping: a rejected open inside a loop must not leak the intermediate
// directory descriptors it opened along the way. It measures the process fd
// count before and after a few hundred rejected walks.
func TestOpenFileNoFDLeakOnRejection(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "a", "b", "c"))
	mustMkdir(t, filepath.Join(dir, "a", "b", "c", "target"))

	// "a/b/c/target" is a directory, so O_WRONLY fails at the final openat
	// after three intermediate descriptors have been opened and chained.
	const iterations = 400
	before := openFDCount(t)
	for range iterations {
		f, err := OpenFile(rootFile, root, "a/b/c/target", os.O_WRONLY|os.O_CREATE, 0o600)
		if err == nil {
			_ = f.Close()
			t.Fatal("OpenFile unexpectedly opened a directory for writing")
		}
	}
	after := openFDCount(t)
	// Allow a small slack for unrelated runtime activity (e.g. the test
	// binary's own logging or netpoll descriptors).
	if after > before+8 {
		t.Fatalf("fd count grew from %d to %d over %d rejected opens: descriptors leaked", before, after, iterations)
	}
}

// openFDCount counts the descriptors held by the current process. It probes
// with fstat rather than reading /proc, which does not exist on macOS.
func openFDCount(t *testing.T) int {
	t.Helper()
	count := 0
	var stat unix.Stat_t
	for fd := 0; fd < 4096; fd++ {
		if err := unix.Fstat(fd, &stat); err == nil {
			count++
		}
	}
	return count
}

func TestOpenFileSymlinkedIntermediateRejected(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "real"))
	mustSymlink(t, "real", filepath.Join(dir, "linkdir"))

	f, err := OpenFile(rootFile, root, "linkdir/file.txt", os.O_WRONLY|os.O_CREATE, 0o600)
	if f != nil {
		_ = f.Close()
		t.Fatal("OpenFile followed a symlinked intermediate directory")
	}
	// O_DIRECTORY|O_NOFOLLOW on a symlink gives ELOOP (mapped to
	// ErrSymlinkWriteTarget) on Linux and ENOTDIR on some kernels/platforms.
	if !errors.Is(err, ErrSymlinkWriteTarget) && !errors.Is(err, unix.ENOTDIR) && !errors.Is(err, unix.ELOOP) {
		t.Fatalf("err = %v, want ErrSymlinkWriteTarget / ENOTDIR / ELOOP", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "real", "file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file created through symlinked directory: Lstat err = %v, want ErrNotExist", err)
	}
}

func TestOpenFileSymlinkedFinalComponentRejected(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustWriteFile(t, filepath.Join(dir, "real.txt"), "original")
	mustSymlink(t, "real.txt", filepath.Join(dir, "link.txt"))

	f, err := OpenFile(rootFile, root, "link.txt", os.O_WRONLY|os.O_TRUNC, 0o600)
	if f != nil {
		_ = f.Close()
		t.Fatal("OpenFile followed a symlinked write target")
	}
	if !errors.Is(err, ErrSymlinkWriteTarget) {
		t.Fatalf("err = %v, want ErrSymlinkWriteTarget", err)
	}
	contents, readErr := os.ReadFile(filepath.Join(dir, "real.txt"))
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(contents) != "original" {
		t.Fatalf("symlink referent was modified: got %q, want %q", contents, "original")
	}
}

// TestOpenFileDanglingSymlinkDoesNotCreateTarget is the classic symlink-write
// attack: a dangling symlink planted at the write target must not cause
// O_CREATE to create (and hand the caller a handle to) the file it points at.
func TestOpenFileDanglingSymlinkDoesNotCreateTarget(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	victim := filepath.Join(dir, "victim.txt")
	mustSymlink(t, "victim.txt", filepath.Join(dir, "bait.txt"))

	f, err := OpenFile(rootFile, root, "bait.txt", os.O_WRONLY|os.O_CREATE, 0o600)
	if err == nil {
		_ = f.Close()
		t.Fatal("OpenFile created through a dangling symlink")
	}
	if !errors.Is(err, ErrSymlinkWriteTarget) {
		t.Fatalf("err = %v, want ErrSymlinkWriteTarget", err)
	}
	if _, statErr := os.Lstat(victim); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("dangling symlink target was created: Lstat err = %v, want ErrNotExist", statErr)
	}
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

	created := filepath.Join(dir, "a", "b", "new.txt")
	contents, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(contents) != "hello" {
		t.Fatalf("contents = %q, want %q", contents, "hello")
	}
	info, err := os.Stat(created)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// perm.Perm() masks to 0777; 0600 survives the usual 022 umask intact.
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %#o, want %#o", got, 0o600)
	}
}

// TestOpenFilePermMaskedToPerm pins that OpenFile passes perm.Perm() — the
// low 9 bits — to openat, so setuid/setgid/sticky bits in the FileMode are
// dropped rather than smuggled onto a newly created file.
func TestOpenFilePermMaskedToPerm(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	f, err := OpenFile(rootFile, root, "sticky.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.ModeSetuid|os.ModeSetgid|0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	_ = f.Close()
	info, err := os.Stat(filepath.Join(dir, "sticky.txt"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		t.Fatalf("mode = %v, setuid/setgid bits leaked through", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %#o, want %#o", got, 0o600)
	}
}

// TestOpenFileEmptyIntermediateComponentSkipped covers the component == ""
// continue branch, which an absolute-looking relPath reaches after Clean
// keeps the leading separator.
func TestOpenFileEmptyIntermediateComponentSkipped(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "a"))

	f, err := OpenFile(rootFile, root, "/a/new.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	_ = f.Close()
	if _, err := os.Stat(filepath.Join(dir, "a", "new.txt")); err != nil {
		t.Fatalf("Stat: %v", err)
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

// ---------------------------------------------------------------------------
// Unlink
// ---------------------------------------------------------------------------

func TestUnlinkNilRootFile(t *testing.T) {
	_, root, _ := newRoot(t)
	err := Unlink(nil, root, "file.txt")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want os.ErrPermission", err)
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

// TestUnlinkDirectoryRejected pins the primitive-level behaviour that
// builtins/rm/rm.go relies on: a directory target is ErrIsDirectory on both
// Linux and macOS, even though macOS unlinkat(2) would happily return nil for
// an empty directory. The fstatat check in front of the unlinkat is what
// makes the two platforms agree.
func TestUnlinkDirectoryRejected(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "empty"))
	mustMkdir(t, filepath.Join(dir, "full", "child"))

	for _, relPath := range []string{"empty", "full"} {
		t.Run(relPath, func(t *testing.T) {
			err := Unlink(rootFile, root, relPath)
			if !errors.Is(err, ErrIsDirectory) {
				t.Fatalf("Unlink(%q) err = %v, want ErrIsDirectory", relPath, err)
			}
			if _, statErr := os.Lstat(filepath.Join(dir, relPath)); statErr != nil {
				t.Fatalf("directory was removed: Lstat err = %v", statErr)
			}
		})
	}
}

func TestUnlinkDirSyntaxOnlyPaths(t *testing.T) {
	// Paths that name the root itself or a parent are always ErrIsDirectory.
	// "../.." is deliberately absent: its first component is an intermediate
	// "..", so it is rejected earlier with os.ErrPermission (covered by
	// TestUnlinkParentTraversalRejected) rather than reaching the base check.
	for _, relPath := range []string{"", ".", "./", "..", "/"} {
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

	for _, relPath := range []string{"../outside.txt", "../../outside.txt"} {
		t.Run("path="+relPath, func(t *testing.T) {
			if err := Unlink(rootFile, root, relPath); !errors.Is(err, os.ErrPermission) {
				t.Fatalf("Unlink(%q) err = %v, want os.ErrPermission", relPath, err)
			}
		})
	}
	if _, err := os.Lstat(outside); err != nil {
		t.Fatalf("file outside the root was removed: %v", err)
	}
}

func TestUnlinkSymlinkedIntermediateRejected(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "real"))
	victim := filepath.Join(dir, "real", "victim.txt")
	mustWriteFile(t, victim, "keep")
	mustSymlink(t, "real", filepath.Join(dir, "linkdir"))

	err := Unlink(rootFile, root, "linkdir/victim.txt")
	if err == nil {
		t.Fatal("Unlink followed a symlinked intermediate directory")
	}
	if !errors.Is(err, ErrSymlinkWriteTarget) && !errors.Is(err, unix.ENOTDIR) && !errors.Is(err, unix.ELOOP) {
		t.Fatalf("err = %v, want ErrSymlinkWriteTarget / ENOTDIR / ELOOP", err)
	}
	if _, statErr := os.Lstat(victim); statErr != nil {
		t.Fatalf("victim removed through symlinked directory: %v", statErr)
	}
}

// TestUnlinkSymlinkFinalComponentRemovesLink pins unlink(2) semantics for the
// leaf: the link itself goes, its referent does not.
func TestUnlinkSymlinkFinalComponentRemovesLink(t *testing.T) {
	t.Run("live symlink", func(t *testing.T) {
		dir, root, rootFile := newRoot(t)
		mustWriteFile(t, filepath.Join(dir, "real.txt"), "keep")
		mustSymlink(t, "real.txt", filepath.Join(dir, "link.txt"))

		if err := Unlink(rootFile, root, "link.txt"); err != nil {
			t.Fatalf("Unlink: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "link.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("link still present: Lstat err = %v, want ErrNotExist", err)
		}
		contents, err := os.ReadFile(filepath.Join(dir, "real.txt"))
		if err != nil {
			t.Fatalf("referent was removed: %v", err)
		}
		if string(contents) != "keep" {
			t.Fatalf("referent contents = %q, want %q", contents, "keep")
		}
	})

	t.Run("dangling symlink", func(t *testing.T) {
		dir, root, rootFile := newRoot(t)
		mustSymlink(t, "nowhere.txt", filepath.Join(dir, "dangling"))

		if err := Unlink(rootFile, root, "dangling"); err != nil {
			t.Fatalf("Unlink: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "dangling")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dangling link still present: Lstat err = %v, want ErrNotExist", err)
		}
	})

	t.Run("self-referential symlink", func(t *testing.T) {
		dir, root, rootFile := newRoot(t)
		mustSymlink(t, "loop", filepath.Join(dir, "loop"))

		if err := Unlink(rootFile, root, "loop"); err != nil {
			t.Fatalf("Unlink: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "loop")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("self-referential link still present: Lstat err = %v, want ErrNotExist", err)
		}
	})

	t.Run("symlink to directory", func(t *testing.T) {
		dir, root, rootFile := newRoot(t)
		mustMkdir(t, filepath.Join(dir, "realdir"))
		mustSymlink(t, "realdir", filepath.Join(dir, "dirlink"))

		// No trailing separator: the link is unlinked, the directory stays.
		if err := Unlink(rootFile, root, "dirlink"); err != nil {
			t.Fatalf("Unlink: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(dir, "dirlink")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("link still present: Lstat err = %v, want ErrNotExist", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "realdir")); err != nil {
			t.Fatalf("referent directory was removed: %v", err)
		}
	})
}

// TestUnlinkTrailingDirSyntax covers the requiresDir / statFlags = 0
// dereference branch: a path that syntactically demands a directory must be
// resolved through any symlink leaf before the directory decision is made.
func TestUnlinkTrailingDirSyntax(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		wantErr error
	}{
		{name: "file with trailing slash", relPath: "file.txt/", wantErr: ErrNotDirectory},
		{name: "file with trailing dot", relPath: "file.txt/.", wantErr: ErrNotDirectory},
		{name: "symlink to file with trailing slash", relPath: "filelink/", wantErr: ErrNotDirectory},
		{name: "symlink to file with trailing dot", relPath: "filelink/.", wantErr: ErrNotDirectory},
		{name: "symlink to dir with trailing slash", relPath: "dirlink/", wantErr: ErrIsDirectory},
		{name: "real dir with trailing slash", relPath: "realdir/", wantErr: ErrIsDirectory},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir, root, rootFile := newRoot(t)
			mustWriteFile(t, filepath.Join(dir, "file.txt"), "keep")
			mustMkdir(t, filepath.Join(dir, "realdir"))
			mustSymlink(t, "file.txt", filepath.Join(dir, "filelink"))
			mustSymlink(t, "realdir", filepath.Join(dir, "dirlink"))

			err := Unlink(rootFile, root, tc.relPath)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Unlink(%q) err = %v, want %v", tc.relPath, err, tc.wantErr)
			}
			// Nothing may be removed on any of these paths.
			for _, name := range []string{"file.txt", "realdir", "filelink", "dirlink"} {
				if _, statErr := os.Lstat(filepath.Join(dir, name)); statErr != nil {
					t.Errorf("%s was removed: %v", name, statErr)
				}
			}
		})
	}
}

// TestUnlinkDanglingSymlinkTrailingSlash: the dereference required by the
// trailing separator cannot resolve, so fstatat fails with ENOENT rather than
// falling back to removing the link.
func TestUnlinkDanglingSymlinkTrailingSlash(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustSymlink(t, "nowhere.txt", filepath.Join(dir, "dangling"))

	err := Unlink(rootFile, root, "dangling/")
	if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ENOTDIR) {
		t.Fatalf("err = %v, want ErrNotExist or ENOTDIR", err)
	}
	if _, statErr := os.Lstat(filepath.Join(dir, "dangling")); statErr != nil {
		t.Fatalf("dangling link was removed: %v", statErr)
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

// TestUnlinkFailsWhenParentNotWritable covers the unlinkat error path after a
// successful fstatat: the target exists and is not a directory, but the
// parent directory denies write.
func TestUnlinkFailsWhenParentNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	dir, root, rootFile := newRoot(t)
	parent := filepath.Join(dir, "ro")
	mustMkdir(t, parent)
	target := filepath.Join(parent, "file.txt")
	mustWriteFile(t, target, "keep")
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	err := Unlink(rootFile, root, "ro/file.txt")
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want os.ErrPermission", err)
	}
	if _, statErr := os.Lstat(target); statErr != nil {
		t.Fatalf("file was removed: %v", statErr)
	}
}

// TestUnlinkNoFDLeak guards the deferred closeDir bookkeeping in the same
// spirit as TestOpenFileNoFDLeakOnRejection, over a create/unlink loop.
func TestUnlinkNoFDLeak(t *testing.T) {
	dir, root, rootFile := newRoot(t)
	mustMkdir(t, filepath.Join(dir, "a", "b"))

	const iterations = 400
	before := openFDCount(t)
	for range iterations {
		mustWriteFile(t, filepath.Join(dir, "a", "b", "tmp.txt"), "x")
		if err := Unlink(rootFile, root, "a/b/tmp.txt"); err != nil {
			t.Fatalf("Unlink: %v", err)
		}
		if err := Unlink(rootFile, root, "a/b/missing.txt"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Unlink(missing) err = %v, want ErrNotExist", err)
		}
	}
	after := openFDCount(t)
	if after > before+8 {
		t.Fatalf("fd count grew from %d to %d over %d unlinks: descriptors leaked", before, after, iterations)
	}
}

// ---------------------------------------------------------------------------
// OpenRoot / CloseRoot
// ---------------------------------------------------------------------------

func TestOpenRootAndCloseRoot(t *testing.T) {
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
	if f == nil {
		t.Fatal("OpenRoot returned a nil file with no error")
	}
	CloseRoot(f)
	// CloseRoot must tolerate nil.
	CloseRoot(nil)
}

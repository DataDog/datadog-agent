// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procfd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath"
)

func TestParseComm(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    string
		wantErr bool
	}{
		{name: "simple", data: "123 (bash) S 1 ...", want: "bash"},
		{name: "trailing newline", data: "123 (bash) S 1 ...\n", want: "bash"},
		{name: "comm with spaces", data: "123 (my program) S 1 ...", want: "my program"},
		{name: "comm with parens", data: "123 (a(b)c) S 1 ...", want: "a(b)c"},
		{name: "no parens", data: "123 bash S 1 ...", wantErr: true},
		{name: "empty", data: "", wantErr: true},
		{name: "unbalanced", data: "123 (bash S 1 ...", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseComm([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseComm(%q) = %q, want error", tt.data, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseComm(%q) unexpected error: %v", tt.data, err)
			}
			if got != tt.want {
				t.Errorf("parseComm(%q) = %q, want %q", tt.data, got, tt.want)
			}
		})
	}
}

func TestFileType(t *testing.T) {
	tests := []struct {
		mode uint32
		want string
	}{
		{unix.S_IFREG, "REG"},
		{unix.S_IFDIR, "DIR"},
		{unix.S_IFCHR, "CHR"},
		{unix.S_IFBLK, "BLK"},
		{unix.S_IFIFO, "FIFO"},
		{unix.S_IFSOCK, "sock"},
		{unix.S_IFLNK, "LINK"},
		{0, "unknown"},
	}
	for _, tt := range tests {
		if got := fileType(tt.mode); got != tt.want {
			t.Errorf("fileType(%#o) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestReadUID(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte("Name:\tbash\nUid:\t1000\t1000\t1000\t1000\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := readUID(dir); got != "1000" {
			t.Errorf("readUID = %q, want %q", got, "1000")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		if got := readUID(dir); got != "?" {
			t.Errorf("readUID = %q, want %q", got, "?")
		}
	})

	t.Run("missing Uid line", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte("Name:\tbash\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := readUID(dir); got != "?" {
			t.Errorf("readUID = %q, want %q", got, "?")
		}
	})
}

func TestIsRealPath(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"/tmp/foo", true},
		{"socket:[12345]", false},
		{"pipe:[12345]", false},
		{"anon_inode:[eventfd]", false},
		{"memfd:secret", false},
		// A real file literally named "/memfd:secret" must still be
		// treated as a path and gated — no "/memfd:" special case.
		{"/memfd:secret", true},
	}
	for _, tt := range tests {
		if got := isRealPath(tt.target); got != tt.want {
			t.Errorf("isRealPath(%q) = %v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestReadFDNames(t *testing.T) {
	t.Run("bounded by max", func(t *testing.T) {
		dir := t.TempDir()
		for i := range 10 {
			f, err := os.Create(filepath.Join(dir, strconv.Itoa(i)))
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
		names, err := readFDNames(dir, 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 3 {
			t.Errorf("readFDNames(dir, 3) returned %d names, want 3", len(names))
		}
	})

	t.Run("max <= 0 returns nothing", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := os.Create(filepath.Join(dir, "0")); err != nil {
			t.Fatal(err)
		}
		names, err := readFDNames(dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 0 {
			t.Errorf("readFDNames(dir, 0) returned %d names, want 0", len(names))
		}
	})

	t.Run("fewer entries than max", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := os.Create(filepath.Join(dir, "0")); err != nil {
			t.Fatal(err)
		}
		names, err := readFDNames(dir, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 {
			t.Errorf("readFDNames(dir, 100) returned %d names, want 1", len(names))
		}
	})
}

// TestListProcessRespectsLimit verifies that listProcess stops once it has
// gathered limit entries rather than reading its full fd table first. This
// is the mechanism that bounds List's aggregate MaxTotalOpenFiles cap.
func TestListProcessRespectsLimit(t *testing.T) {
	pid := os.Getpid()

	files, err := listProcess(procpath.Default, pid, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 2 {
		t.Errorf("listProcess(_, _, 2) returned %d entries, want <= 2", len(files))
	}

	files, err = listProcess(procpath.Default, pid, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("listProcess(_, _, 0) returned %d entries, want 0", len(files))
	}
}

// TestListSelf exercises List against the real, live /proc of the current
// test process. Synthetic /proc fixtures cannot reproduce the kernel's
// "(deleted)" magic-symlink behaviour (a plain filesystem symlink whose
// target string has " (deleted)" appended does not resolve, so unix.Stat
// would fail where the real /proc/<pid>/fd entry succeeds) — only a real
// Linux kernel can exhibit that. This test therefore opens a real file,
// deletes it while held open, and confirms the observed behaviour end to
// end.
func TestListSelf(t *testing.T) {
	pid := os.Getpid()

	dir := t.TempDir()
	path := filepath.Join(dir, "openfile")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	ctx := context.Background()
	files, err := List(ctx, procpath.Default, []int{pid}, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var found *OpenFile
	wantFD := strconv.Itoa(int(f.Fd()))
	var sawCwd, sawTxt bool
	for i := range files {
		of := &files[i]
		if of.PID != pid {
			t.Errorf("List returned entry for pid %d, want only %d", of.PID, pid)
		}
		switch of.FD {
		case "cwd":
			sawCwd = true
		case "txt":
			sawTxt = true
		case wantFD:
			found = of
		}
	}
	if !sawCwd {
		t.Error("List did not return a cwd entry")
	}
	if !sawTxt {
		t.Error("List did not return a txt entry")
	}
	if found == nil {
		t.Fatalf("List did not return an entry for fd %s (%s)", wantFD, path)
	}
	if found.Name != path {
		t.Errorf("open file Name = %q, want %q", found.Name, path)
	}
	if found.Type != "REG" {
		t.Errorf("open file Type = %q, want REG", found.Type)
	}
	if found.Deleted {
		t.Error("open file reported Deleted before removal")
	}
	if !found.IsPath {
		t.Error("open file reported IsPath=false for a real path")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	files, err = List(ctx, procpath.Default, []int{pid}, nil)
	if err != nil {
		t.Fatalf("List after removal: %v", err)
	}
	found = nil
	for i := range files {
		if files[i].FD == wantFD {
			found = &files[i]
		}
	}
	if found == nil {
		t.Fatalf("List did not return an entry for fd %s after removal", wantFD)
	}
	if !found.Deleted {
		t.Error("open file did not report Deleted after removal")
	}
	if found.Name != path {
		t.Errorf("open file Name after removal = %q, want %q (deleted suffix must be stripped)", found.Name, path)
	}
}

// TestListSelfLiteralDeletedSuffixTreatedAsDeletionMarker opens a real,
// never-deleted file whose actual on-disk name literally ends in
// " (deleted)" (a string an attacker fully controls when creating a file)
// and confirms readLinkEntry always treats that suffix as a kernel deletion
// marker: Name has the suffix stripped and Deleted is true. Disambiguating
// this case correctly would require stat'ing the process-controlled target
// path itself, which docs/RULES.md's "File Access — Safe Wrappers Only"
// rule forbids (this package's AllowedPaths exception covers only
// hardcoded /proc reads); trusting the suffix unconditionally is the same
// behaviour real lsof/ls/readlink exhibit, and is an accepted limitation
// rather than a bug.
func TestListSelfLiteralDeletedSuffixTreatedAsDeletionMarker(t *testing.T) {
	pid := os.Getpid()

	dir := t.TempDir()
	path := filepath.Join(dir, "openfile (deleted)")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	ctx := context.Background()
	files, err := List(ctx, procpath.Default, []int{pid}, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantFD := strconv.Itoa(int(f.Fd()))
	var found *OpenFile
	for i := range files {
		if files[i].FD == wantFD {
			found = &files[i]
		}
	}
	if found == nil {
		t.Fatalf("List did not return an entry for fd %s (%s)", wantFD, path)
	}
	wantName := strings.TrimSuffix(path, " (deleted)")
	if found.Name != wantName {
		t.Errorf("open file Name = %q, want %q (kernel deletion-marker suffix stripped)", found.Name, wantName)
	}
	if !found.Deleted {
		t.Error("open file reported Deleted=false for a target ending in the kernel's \" (deleted)\" marker string")
	}
}

// TestResolvePIDsValidatesProcPathWithExplicitPIDs verifies that an
// inaccessible procPath surfaces as a real error even when the caller
// supplies an explicit -p PID list. Before this check existed, only the
// auto-discovery path (no -p) validated procPath (via os.ReadDir); an
// explicit PID list bypassed that validation entirely and silently returned
// a lookup that would later fail per-process, rather than reporting the
// broken ProcPath configuration itself.
func TestResolvePIDsValidatesProcPathWithExplicitPIDs(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := resolvePIDs(context.Background(), missing, []int{1}); err == nil {
		t.Error("resolvePIDs with an inaccessible procPath and explicit PIDs should return an error")
	}
}

// TestResolvePIDsRejectsNonDirectoryProcPathWithExplicitPIDs verifies that a
// procPath which exists but is a regular file (not a directory) is rejected
// up front with an explicit PIDs list, rather than passing os.Stat's mere
// existence check and then silently skipping every per-PID read (each of
// which would fail to open procPath/<pid>/fd as a directory), leaving the
// caller with an empty result and no diagnosable error.
func TestResolvePIDsRejectsNonDirectoryProcPathWithExplicitPIDs(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolvePIDs(context.Background(), notADir, []int{1}); err == nil {
		t.Error("resolvePIDs with a non-directory procPath and explicit PIDs should return an error")
	}
}

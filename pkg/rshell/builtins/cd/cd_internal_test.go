// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cd

import (
	"context"
	iofs "io/fs"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// absRoot is an absolute root suitable for tests on the host platform:
// "/" on Unix and `C:\` on Windows.
func absRoot() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}
	return string(filepath.Separator)
}

// --- rootPrefix ---

func TestRootPrefixUnixRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	assert.Equal(t, "/", rootPrefix("/"))
	assert.Equal(t, "/", rootPrefix("/a/b"))
	assert.Equal(t, "/", rootPrefix("/a"))
}

func TestRootPrefixRelative(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	assert.Equal(t, "/", rootPrefix("a/b"))
}

// --- parentDir ---

func TestParentDirUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	assert.Equal(t, "/", parentDir("/"), "root has no parent — stays at root")
	assert.Equal(t, "/", parentDir("/a"))
	assert.Equal(t, "/a", parentDir("/a/b"))
	assert.Equal(t, "/a/b", parentDir("/a/b/c"))
}

// --- joinPath ---

func TestJoinPathHandlesTrailingSeparator(t *testing.T) {
	root := absRoot()
	assert.Equal(t, root+"a", joinPath(root, "a"), "no double separator after root")
	assert.Equal(t, filepath.Join(root, "a")+string(filepath.Separator)+"b",
		joinPath(filepath.Join(root, "a"), "b"))
}

func TestJoinPathEmptyDir(t *testing.T) {
	assert.Equal(t, "comp", joinPath("", "comp"))
}

// --- boolSeqFlag ---

func TestBoolSeqFlagTypeIsBool(t *testing.T) {
	seq := 0
	f := newBoolSeqFlag(&seq)
	assert.Equal(t, "bool", f.Type())
}

func TestBoolSeqFlagStringIsFalse(t *testing.T) {
	seq := 0
	f := newBoolSeqFlag(&seq)
	assert.Equal(t, "false", f.String())
}

func TestBoolSeqFlagSetSentinelOnly(t *testing.T) {
	seq := 0
	f := newBoolSeqFlag(&seq)
	assert.NoError(t, f.Set(boolSeqSentinel))
	assert.Equal(t, 1, f.pos)

	// A second value increments.
	g := newBoolSeqFlag(&seq)
	assert.NoError(t, g.Set(boolSeqSentinel))
	assert.Equal(t, 2, g.pos)
}

func TestBoolSeqFlagRejectsExplicitValue(t *testing.T) {
	seq := 0
	f := newBoolSeqFlag(&seq)
	err := f.Set("true")
	assert.Error(t, err)
	err = f.Set("false")
	assert.Error(t, err)
	err = f.Set("")
	assert.Error(t, err)
}

// --- HostPrefix re-prefix hardening (resolveSymlinks branch) ---

// fakeStatInfo is a minimal fs.FileInfo implementation for stub
// LstatFile / StatFile responses.
type fakeStatInfo struct {
	name   string
	mode   iofs.FileMode
	dir    bool
	isLink bool
}

func (f fakeStatInfo) Name() string {
	if f.name != "" {
		return f.name
	}
	return "stub"
}
func (f fakeStatInfo) Size() int64 { return 0 }
func (f fakeStatInfo) Mode() iofs.FileMode {
	if f.isLink {
		return f.mode | iofs.ModeSymlink
	}
	if f.dir {
		return f.mode | iofs.ModeDir
	}
	return f.mode
}
func (f fakeStatInfo) ModTime() time.Time { return time.Time{} }
func (f fakeStatInfo) IsDir() bool        { return f.dir }
func (f fakeStatInfo) Sys() any           { return nil }

// makeHostPrefixCallCtx returns a *builtins.CallContext stubbed so that
// resolvePath in -P mode can walk a single symlink at /sandbox/link and
// observe the HostPrefix branch.
//
//   - /sandbox itself is a directory (above-sandbox passthrough not
//     exercised since we hand sandbox-absolute paths in).
//   - /sandbox/link is a symlink whose target is supplied by the
//     caller (linkTarget).
//   - HostPrefix returns hp.
//
// The walker only invokes LstatFile / ReadlinkFile / HostPrefix; the
// returned `out` is the post-walker path with any HostPrefix re-prefix
// applied. StatFile is stubbed permissively because the walker does
// not call it for plain appends in -P mode.
func makeHostPrefixCallCtx(t *testing.T, linkPath, linkTarget, hp string) *builtins.CallContext {
	t.Helper()
	return &builtins.CallContext{
		StatFile: func(_ context.Context, _ string) (iofs.FileInfo, error) {
			return fakeStatInfo{dir: true}, nil
		},
		LstatFile: func(_ context.Context, path string) (iofs.FileInfo, error) {
			if path == linkPath {
				return fakeStatInfo{isLink: true}, nil
			}
			return fakeStatInfo{dir: true}, nil
		},
		ReadlinkFile: func(_ context.Context, path string) (string, error) {
			if path == linkPath {
				return linkTarget, nil
			}
			return "", &iofs.PathError{}
		},
		HostPrefix: func() string { return hp },
	}
}

// TestHostPrefixRePrefixesUnrelatedAbsoluteTarget verifies that when a
// symlink target's absolute path does NOT already start with the
// configured host prefix, the walker prepends it. This is the common
// container case: host symlinks reference /var/log/... which is only
// reachable through /mnt/host/var/log/... inside the sandbox.
func TestHostPrefixRePrefixesUnrelatedAbsoluteTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	callCtx := makeHostPrefixCallCtx(t, "/sandbox/link", "/var/log/data", "/mnt/host")
	got, err := resolvePath(context.Background(), callCtx, "/sandbox/link", true)
	assert.NoError(t, err)
	assert.Equal(t, "/mnt/host/var/log/data", got,
		"absolute target without the host prefix must be re-prefixed")
}

// TestHostPrefixSkipsRePrefixWhenAlreadyPrefixed verifies that a target
// already inside the host prefix tree (hp + sep + tail) is not double-
// prefixed.
func TestHostPrefixSkipsRePrefixWhenAlreadyPrefixed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	callCtx := makeHostPrefixCallCtx(t, "/sandbox/link", "/mnt/host/var/log/data", "/mnt/host")
	got, err := resolvePath(context.Background(), callCtx, "/sandbox/link", true)
	assert.NoError(t, err)
	assert.Equal(t, "/mnt/host/var/log/data", got,
		"a target already under the host prefix must not be re-prefixed")
}

// TestHostPrefixSkipsRePrefixWhenExactlyTheHostPrefix verifies that a
// symlink target equal to the host prefix itself (no trailing tail) is
// not re-prefixed onto itself ("/mnt/host/mnt/host").
func TestHostPrefixSkipsRePrefixWhenExactlyTheHostPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	callCtx := makeHostPrefixCallCtx(t, "/sandbox/link", "/mnt/host", "/mnt/host")
	got, err := resolvePath(context.Background(), callCtx, "/sandbox/link", true)
	assert.NoError(t, err)
	assert.Equal(t, "/mnt/host", got,
		"target exactly equal to the host prefix must not be re-prefixed")
}

// TestHostPrefixRePrefixesPathPrefixCollision is the key contract test
// flagged in code review (P2): a target like `/mnt/hostile/foo` shares
// a substring with the host prefix `/mnt/host` but is a different
// directory. The walker uses `hp + sep` to detect the boundary, so the
// collision target gets re-prefixed to `/mnt/host/mnt/hostile/foo`.
// The final sandbox.Stat in changeDir is responsible for rejecting any
// result outside the sandbox; this test locks the re-prefix logic into
// place so a regression in the boundary check would surface here.
func TestHostPrefixRePrefixesPathPrefixCollision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path semantics")
	}
	callCtx := makeHostPrefixCallCtx(t, "/sandbox/link", "/mnt/hostile/foo", "/mnt/host")
	got, err := resolvePath(context.Background(), callCtx, "/sandbox/link", true)
	assert.NoError(t, err)
	assert.Equal(t, "/mnt/host/mnt/hostile/foo", got,
		"hp+sep boundary must distinguish /mnt/host from /mnt/hostile and re-prefix the latter")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package pwd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// pwdRunDirAllowed runs a script with `dir` as the working directory and
// `allowedRoot` as the AllowedPaths root. Useful when the cwd is *inside*
// a larger sandbox (so the resolver can walk above the cwd, exercising
// real -P logic) — unlike pwdRun in pwd_test.go which scopes AllowedPaths
// to the cwd itself.
func pwdRunDirAllowed(t *testing.T, script, dir, allowedRoot string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{allowedRoot}))
}

// TestPwdPhysicalResolvesSymlink: when the cwd is reached via a symlink
// inside the sandbox, "pwd -P" must print the canonical (target) path.
func TestPwdPhysicalResolvesSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, os.Symlink(target, link))

	stdoutP, stderr, code := pwdRunDirAllowed(t, "pwd -P", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, target+"\n", stdoutP)
}

// TestPwdLogicalKeepsSymlink: when the cwd is reached via a symlink,
// "pwd -L" must print the logical (symlink) path, not the canonical one.
func TestPwdLogicalKeepsSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, os.Symlink(target, link))

	stdoutL, _, code := pwdRunDirAllowed(t, "pwd -L", link, root)
	require.Equal(t, 0, code)
	assert.Equal(t, link+"\n", stdoutL)
}

// TestPwdLastWinsPThenLWithSymlink: with a symlinked cwd, "pwd -P -L"
// must emit the logical (symlinked) path because -L appears last on
// the command line. This regression-tests the bug where pflag's Visit
// walks flags in lexicographical order rather than command-line order
// — without the boolSeqFlag pos-tracking, the wrong mode is selected
// even though both flags are present.
func TestPwdLastWinsPThenLWithSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, os.Symlink(target, link))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P -L", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, link+"\n", stdout, "-P -L: -L wins, must emit logical path")
}

// TestPwdLastWinsLThenPWithSymlink: the mirror case — -L then -P picks
// physical (the resolved target).
func TestPwdLastWinsLThenPWithSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, os.Symlink(target, link))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -L -P", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, target+"\n", stdout, "-L -P: -P wins, must emit physical path")
}

// TestPwdPhysicalChainedSymlinks: A -> B -> C resolves to C.
func TestPwdPhysicalChainedSymlinks(t *testing.T) {
	root := canonicalTempDir(t)
	c := filepath.Join(root, "c")
	require.NoError(t, os.Mkdir(c, 0755))
	b := filepath.Join(root, "b")
	require.NoError(t, os.Symlink(c, b))
	a := filepath.Join(root, "a")
	require.NoError(t, os.Symlink(b, a))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P", a, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, c+"\n", stdout)
}

// TestPwdPhysicalRelativeSymlink: relative symlink targets are resolved
// against the link's directory, not the cwd.
func TestPwdPhysicalRelativeSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(root, "real"), 0755))
	// "lnk" → "real" (relative). When at $root/lnk, -P must yield $root/real.
	require.NoError(t, os.Symlink("real", filepath.Join(root, "lnk")))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P", filepath.Join(root, "lnk"), root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, filepath.Join(root, "real")+"\n", stdout)
}

// TestPwdPhysicalSymlinkCycle: A -> B -> A loops; -P must error with a
// loop diagnostic and exit 1, not hang or recurse forever.
func TestPwdPhysicalSymlinkCycle(t *testing.T) {
	root := canonicalTempDir(t)
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	require.NoError(t, os.Symlink(b, a))
	require.NoError(t, os.Symlink(a, b))

	_, stderr, code := pwdRunDirAllowed(t, "pwd -P", a, root)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "pwd:")
	assert.Contains(t, stderr, "too many levels of symbolic links")
}

// TestPwdPhysicalNestedSymlinks: link inside a real dir, with components
// after the link.  $root/real/lnk → $root/real/sub.
func TestPwdPhysicalNestedSymlinks(t *testing.T) {
	root := canonicalTempDir(t)
	realDir := filepath.Join(root, "real")
	sub := filepath.Join(realDir, "sub")
	require.NoError(t, os.MkdirAll(sub, 0755))
	link := filepath.Join(realDir, "lnk")
	require.NoError(t, os.Symlink("sub", link))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, sub+"\n", stdout)
}

// TestPwdPhysicalWithDotDotInTarget: a symlink whose target contains
// ".." resolves correctly. $root/sibling/lnk → "../target".
func TestPwdPhysicalWithDotDotInTarget(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "target")
	sibling := filepath.Join(root, "sibling")
	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, os.Mkdir(sibling, 0755))
	link := filepath.Join(sibling, "lnk")
	require.NoError(t, os.Symlink("../target", link))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, target+"\n", stdout)
}

// TestPwdPhysicalAbsoluteSymlinkTargetInsideSandbox: an absolute symlink
// whose target is inside the sandbox resolves correctly (target is
// absolute, so we reset the resolved prefix to the absolute root).
func TestPwdPhysicalAbsoluteSymlinkTargetInsideSandbox(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "abslnk")
	require.NoError(t, os.Mkdir(target, 0755))
	require.NoError(t, os.Symlink(target, link)) // absolute path

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, target+"\n", stdout)
}

// TestPwdPhysicalDotDotComponents: symlink target with explicit ".."
// segments is canonicalized.  $root/d1/d2/lnk → "../../d3", which lands
// at $root/d3.
func TestPwdPhysicalDotDotResolvesAcrossDepth(t *testing.T) {
	root := canonicalTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "d1", "d2"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(root, "d3"), 0755))
	link := filepath.Join(root, "d1", "d2", "lnk")
	require.NoError(t, os.Symlink("../../d3", link))

	stdout, stderr, code := pwdRunDirAllowed(t, "pwd -P", link, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, filepath.Join(root, "d3")+"\n", stdout)
}

// TestPwdPhysicalAppliesHostPrefixToAbsoluteSymlinkTarget: in a
// container-style sandbox where AllowedPaths roots live under a host
// mount prefix and on-disk symlinks store host-absolute targets,
// `pwd -P` must apply the HostPrefix so the printed path is reachable
// through the sandbox. Without the prefix, the output is the literal
// readlink string (e.g. /var/log/pods/app), which the user cannot
// access via further filesystem operations.
//
// Layout:
//
//	$root/host/var/log/pods/app/        (real dir)
//	$root/host/var/log/containers/app   (symlink to /var/log/pods/app)
//
// HostPrefix = $root/host. AllowedPaths = $root/host/var/log/.
// cd into containers/app, then `pwd -P` must emit
// $root/host/var/log/pods/app, not /var/log/pods/app.
func TestPwdPhysicalAppliesHostPrefixToAbsoluteSymlinkTarget(t *testing.T) {
	root := canonicalTempDir(t)
	hostPrefix := filepath.Join(root, "host")
	pods := filepath.Join(hostPrefix, "var", "log", "pods", "app")
	containers := filepath.Join(hostPrefix, "var", "log", "containers")
	require.NoError(t, os.MkdirAll(pods, 0755))
	require.NoError(t, os.MkdirAll(containers, 0755))
	link := filepath.Join(containers, "app")
	// Host-absolute target without the prefix — typical of container
	// log directories where pods/containers are bind-mounted from the
	// host filesystem.
	require.NoError(t, os.Symlink("/var/log/pods/app", link))

	allowedRoot := filepath.Join(hostPrefix, "var", "log")
	stdout, stderr, code := testutil.RunScript(t, "pwd -P", link,
		interp.AllowedPaths([]string{allowedRoot}),
		interp.HostPrefix(hostPrefix),
	)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, pods+"\n", stdout, "host-absolute symlink target must be prefixed with HostPrefix")
}

// TestPwdPhysicalSkipsHostPrefixWhenAlreadyApplied: if the resolved
// target already begins with the host prefix (e.g. a relative symlink
// stayed within the prefixed tree), HostPrefix should not be applied
// again.
func TestPwdPhysicalSkipsHostPrefixWhenAlreadyApplied(t *testing.T) {
	root := canonicalTempDir(t)
	hostPrefix := filepath.Join(root, "host")
	target := filepath.Join(hostPrefix, "real")
	link := filepath.Join(hostPrefix, "lnk")
	require.NoError(t, os.MkdirAll(target, 0755))
	// Absolute target already includes the host prefix — must not be
	// double-prefixed.
	require.NoError(t, os.Symlink(target, link))

	stdout, stderr, code := testutil.RunScript(t, "pwd -P", link,
		interp.AllowedPaths([]string{hostPrefix}),
		interp.HostPrefix(hostPrefix),
	)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, target+"\n", stdout)
}

// TestPwdPhysicalSymlinkedSandboxRoot: when the AllowedPaths root is
// itself a symlink (e.g. /tmp/link -> /tmp/real), `pwd -P` must emit
// the canonical (resolved) path. The sandbox follows the root symlink
// at os.OpenRoot time, so the per-component LstatFile walk doesn't
// see it; the CanonicalizeRootPrefix accessor restores the missing
// resolution.
func TestPwdPhysicalSymlinkedSandboxRoot(t *testing.T) {
	parent := canonicalTempDir(t)
	realRoot := filepath.Join(parent, "real")
	linkRoot := filepath.Join(parent, "link")
	require.NoError(t, os.MkdirAll(filepath.Join(realRoot, "sub"), 0755))
	// /parent/link -> /parent/real
	require.NoError(t, os.Symlink(realRoot, linkRoot))

	// cwd is the symlinked root + a subdir; AllowedPaths is the
	// symlinked root (the realistic case described by Codex's review).
	cwd := filepath.Join(linkRoot, "sub")
	stdout, stderr, code := testutil.RunScript(t, "pwd -P", cwd,
		interp.AllowedPaths([]string{linkRoot}),
	)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	want := filepath.Join(realRoot, "sub") + "\n"
	assert.Equal(t, want, stdout, "pwd -P must resolve a symlinked AllowedPaths root")
}

// TestPwdPhysicalNonSymlinkedSandboxRootUnchanged: control case — when
// the AllowedPaths root is not a symlink, the resolved path is the
// configured path verbatim.
func TestPwdPhysicalNonSymlinkedSandboxRootUnchanged(t *testing.T) {
	root := canonicalTempDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0755))

	cwd := filepath.Join(root, "sub")
	stdout, stderr, code := testutil.RunScript(t, "pwd -P", cwd,
		interp.AllowedPaths([]string{root}),
	)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, cwd+"\n", stdout)
}

// TestPwdPhysicalHostPrefixWithTrailingSlash: a HostPrefix passed with
// a trailing separator must be normalized before pwd -P uses it for
// prefix-matching. Without normalization, a target that already
// includes the cleaned prefix would fail the HasPrefix check (because
// "$root/host/" + "/" != "$root/host//") and get doubled — emitting
// `$root/host/$root/host/real`. The CallContext.HostPrefix accessor
// returns the sandbox's filepath.Clean'd prefix, so this test pins
// the contract.
func TestPwdPhysicalHostPrefixWithTrailingSlash(t *testing.T) {
	root := canonicalTempDir(t)
	hostPrefix := filepath.Join(root, "host")
	target := filepath.Join(hostPrefix, "real")
	link := filepath.Join(hostPrefix, "lnk")
	require.NoError(t, os.MkdirAll(target, 0755))
	require.NoError(t, os.Symlink(target, link))

	// Pass HostPrefix with a trailing slash on purpose — this is the
	// kind of input that previously caused double-prefixing.
	stdout, stderr, code := testutil.RunScript(t, "pwd -P", link,
		interp.AllowedPaths([]string{hostPrefix}),
		interp.HostPrefix(hostPrefix+string(filepath.Separator)),
	)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, target+"\n", stdout, "trailing-slash HostPrefix must not produce a doubled-prefix path")
}

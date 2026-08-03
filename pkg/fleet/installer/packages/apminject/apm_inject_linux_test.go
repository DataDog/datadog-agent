// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apminject

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crasherSO builds and returns the path to a minimal shared library whose
// constructor performs a null-pointer write (SIGSEGV) on load.
// It simulates a buggy APM injector that would take down every process on
// the host if written to /etc/ld.so.preload.
//
// Compilation requires gcc; the test is skipped if it is not available.
// -nodefaultlibs / -nostdlib ensure no DT_NEEDED entries so the .so loads
// cleanly on the host without any extra runtime libraries.
func crasherSO(t *testing.T, dir string) string {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available; skipping crasher-library test")
	}
	src := filepath.Join(dir, "crash.c")
	so := filepath.Join(dir, "crash.so")
	code := `static void __attribute__((constructor)) crash(void) { volatile char *p = (volatile char *)0; *p = 0; }`
	require.NoError(t, os.WriteFile(src, []byte(code), 0644))
	out, err := exec.Command("gcc", "-shared", "-fPIC", "-nodefaultlibs", "-nostdlib", "-o", so, src).CombinedOutput()
	require.NoError(t, err, "failed to compile crasher library: %s", out)
	return so
}

// TestVerifySharedLib_BuggyLibrary verifies that verifySharedLib rejects a
// shared library whose constructor crashes on load.
func TestVerifySharedLib_BuggyLibrary(t *testing.T) {
	tmpDir := t.TempDir()
	so := crasherSO(t, tmpDir)

	a := &InjectorInstaller{installPath: tmpDir}
	err := a.verifySharedLib(context.TODO(), so)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify injected lib")
}

// TestInstrumentLDPreload_BuggyLibrary verifies that InstrumentLDPreload
// refuses to modify ld.so.preload when the launcher library crashes on load.
// This is the core safety gate: a broken launcher must never be written to
// ld.so.preload where it would affect every process on the host.
func TestInstrumentLDPreload_BuggyLibrary(t *testing.T) {
	tmpDir := t.TempDir()
	injectDir := filepath.Join(tmpDir, "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0755))

	// Place the crasher at the launcher path that InstrumentLDPreload checks.
	launcherPath := filepath.Join(injectDir, "launcher.preload.so")
	so := crasherSO(t, tmpDir)
	require.NoError(t, os.Rename(so, launcherPath))

	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	a := newInstallerWithPaths(tmpDir, preloadFile)

	err := a.InstrumentLDPreload(context.TODO(), ViaPersistentPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify injected lib")

	// ld.so.preload must not have been created — the sanity check runs first.
	_, statErr := os.Stat(preloadFile)
	assert.True(t, os.IsNotExist(statErr), "ld.so.preload must not be created when the launcher is buggy")
}

// TestInstrumentLDPreload_AlreadyInstalled verifies that InstrumentLDPreload
// is idempotent: running it twice with a valid launcher leaves ld.so.preload
// with exactly one entry (no duplicates).
func TestInstrumentLDPreload_AlreadyInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	injectDir := filepath.Join(tmpDir, "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0755))

	// Create a real (harmless) shared library so verifySharedLib succeeds.
	launcherPath := filepath.Join(injectDir, "launcher.preload.so")
	require.NoError(t, buildHarmlessSO(t, tmpDir, launcherPath))

	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	// Pre-populate with the launcher path, as if it were already installed.
	require.NoError(t, os.WriteFile(preloadFile, []byte(launcherPath+"\n"), 0644))

	a := newInstallerWithPaths(tmpDir, preloadFile)

	err := a.InstrumentLDPreload(context.TODO(), ViaPersistentPath)
	assert.NoError(t, err)

	content, err := os.ReadFile(preloadFile)
	require.NoError(t, err)
	// The path should appear exactly once.
	count := 0
	for _, line := range splitLines(string(content)) {
		if line == launcherPath {
			count++
		}
	}
	assert.Equal(t, 1, count, "launcher path must appear exactly once in ld.so.preload")
}

// TestUninstrumentLDPreload_WithActiveBuggyPreload verifies that
// UninstrumentLDPreload succeeds even when ld.so.preload references a
// library that would crash any dynamically-linked binary on load.
//
// UninstrumentLDPreload performs only file I/O (no subprocess execution), so
// it is never affected by whatever is in ld.so.preload.  The production
// installer binary additionally carries the static-linking guarantee
// (CGO_ENABLED=0, tags osusergo,netgo) so the process itself is immune to a
// broken preload.
func TestUninstrumentLDPreload_WithActiveBuggyPreload(t *testing.T) {
	tmpDir := t.TempDir()
	injectDir := filepath.Join(tmpDir, "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0755))

	launcherPath := filepath.Join(injectDir, "launcher.preload.so")
	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	a := newInstallerWithPaths(tmpDir, preloadFile)

	// Step 1: instrument with a harmless library — verifySharedLib passes and
	// the launcher path is written to ld.so.preload.
	require.NoError(t, buildHarmlessSO(t, tmpDir, launcherPath))
	require.NoError(t, a.InstrumentLDPreload(context.TODO(), ViaPersistentPath))

	content, err := os.ReadFile(preloadFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "launcher.preload.so")

	// Step 2: swap the harmless library for a crashing one, simulating a
	// corrupted or buggy injector that was deployed after initial installation.
	so := crasherSO(t, tmpDir)
	require.NoError(t, os.Rename(so, launcherPath))

	// Step 3: uninstrument — must succeed even though the library at
	// launcherPath now crashes on load.  UninstrumentLDPreload performs only
	// file I/O and never spawns a subprocess, so it is immune to the broken
	// preload entry.
	require.NoError(t, a.UninstrumentLDPreload(context.TODO()))

	// Step 4: the launcher entry must be gone from ld.so.preload.
	content, err = os.ReadFile(preloadFile)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "launcher.preload.so",
		"launcher entry must be removed from ld.so.preload even when the library is buggy")
}

// TestInstrumentLDPreload_TmpfsSymlink verifies that ViaTmpfsLink references the
// launcher through the tmpfs symlink: it creates the symlink pointing at the
// persistent payload and writes the tmpfs path (not the persistent path) to
// ld.so.preload.
func TestInstrumentLDPreload_TmpfsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	injectDir := filepath.Join(tmpDir, "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0755))

	launcherPath := filepath.Join(injectDir, "launcher.preload.so")
	require.NoError(t, buildHarmlessSO(t, tmpDir, launcherPath))

	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	tmpfsDir := filepath.Join(tmpDir, "run-link")

	a := newInstallerWithPaths(tmpDir, preloadFile)
	a.tmpfsInjectDir = tmpfsDir

	require.NoError(t, a.InstrumentLDPreload(context.TODO(), ViaTmpfsLink))

	// The symlink resolves to the persistent payload (so AppArmor and the
	// launcher's sibling resolution keep working through /opt).
	resolved, err := os.Readlink(tmpfsDir)
	require.NoError(t, err)
	assert.Equal(t, injectDir, resolved)

	content, err := os.ReadFile(preloadFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), filepath.Join(tmpfsDir, "launcher.preload.so"))
	assert.NotContains(t, string(content), launcherPath,
		"ld.so.preload must reference the tmpfs path, not the persistent path")
}

// TestInstrumentLDPreload_TmpfsBrokenDoesNotLink verifies that a broken launcher
// is never reachable through the symlink: verification happens before the
// symlink is created, so on failure neither the symlink nor the ld.so.preload
// entry is written.
func TestInstrumentLDPreload_TmpfsBrokenDoesNotLink(t *testing.T) {
	tmpDir := t.TempDir()
	injectDir := filepath.Join(tmpDir, "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0755))

	launcherPath := filepath.Join(injectDir, "launcher.preload.so")
	so := crasherSO(t, tmpDir)
	require.NoError(t, os.Rename(so, launcherPath))

	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	tmpfsDir := filepath.Join(tmpDir, "run-link")

	a := newInstallerWithPaths(tmpDir, preloadFile)
	a.tmpfsInjectDir = tmpfsDir

	err := a.InstrumentLDPreload(context.TODO(), ViaTmpfsLink)
	assert.Error(t, err)

	_, statErr := os.Lstat(tmpfsDir)
	assert.True(t, os.IsNotExist(statErr), "symlink must not be created when the launcher is buggy")
	_, statErr = os.Stat(preloadFile)
	assert.True(t, os.IsNotExist(statErr), "ld.so.preload must not be created when the launcher is buggy")
}

// TestInstrumentLDPreload_TmpfsModeNoPersistentEntry reproduces the install flow
// on a systemd-managed host: the systemd service first writes the tmpfs entry
// (InstrumentLDPreload with ViaTmpfsLink), then the installer's own direct write
// runs over the same ld.so.preload. With ViaTmpfsLink propagated to the direct
// write, that second write must stay on the tmpfs path and must NOT append the
// persistent /opt path — otherwise a broken launcher would still load from the
// persistent path on the next reboot, defeating the tmpfs safety guarantee.
func TestInstrumentLDPreload_TmpfsModeNoPersistentEntry(t *testing.T) {
	tmpDir := t.TempDir()
	injectDir := filepath.Join(tmpDir, "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0755))

	launcherPath := filepath.Join(injectDir, "launcher.preload.so")
	require.NoError(t, buildHarmlessSO(t, tmpDir, launcherPath))

	preloadFile := filepath.Join(tmpDir, "ld.so.preload")
	tmpfsDir := filepath.Join(tmpDir, "run-link")
	tmpfsLauncher := filepath.Join(tmpfsDir, "launcher.preload.so")

	// Step 1: the systemd service process writes the tmpfs entry.
	svc := newInstallerWithPaths(tmpDir, preloadFile)
	svc.tmpfsInjectDir = tmpfsDir
	require.NoError(t, svc.InstrumentLDPreload(context.TODO(), ViaTmpfsLink))

	// Step 2: the installer process does its own direct write over the same
	// file. On a systemd-managed host Instrument passes ViaTmpfsLink to this
	// call, so it must remain on the tmpfs path.
	inst := newInstallerWithPaths(tmpDir, preloadFile)
	inst.tmpfsInjectDir = tmpfsDir
	require.NoError(t, inst.InstrumentLDPreload(context.TODO(), ViaTmpfsLink))

	content, err := os.ReadFile(preloadFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), tmpfsLauncher,
		"ld.so.preload must reference the tmpfs path")
	assert.NotContains(t, string(content), launcherPath,
		"ld.so.preload must not contain the persistent /opt launcher path")
	// The tmpfs entry must appear exactly once (idempotent across both writes).
	count := 0
	for _, line := range splitLines(string(content)) {
		if line == tmpfsLauncher {
			count++
		}
	}
	assert.Equal(t, 1, count, "tmpfs launcher path must appear exactly once")
}

// buildHarmlessSO compiles a shared library that does nothing on load.
// Returns an error if gcc is not available (caller should skip the test).
func buildHarmlessSO(t *testing.T, dir, dst string) error {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("gcc not available; skipping harmless-SO test")
	}
	src := filepath.Join(dir, "harmless.c")
	require.NoError(t, os.WriteFile(src, []byte(`void noop(void) {}`), 0644))
	out, err := exec.Command("gcc", "-shared", "-fPIC", "-o", dst, src).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to compile harmless library: %s: %v", out, err)
	}
	return nil
}

// splitLines splits a string into non-empty, trimmed lines.
func splitLines(s string) []string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

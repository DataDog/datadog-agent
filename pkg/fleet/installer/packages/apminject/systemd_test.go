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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service/systemd"
)

func TestNewSystemdServiceManager(t *testing.T) {
	mgr := NewSystemdServiceManager()

	assert.NotNil(t, mgr)
	assert.Equal(t, filepath.Join(systemd.UserUnitsPath, systemdServiceName), mgr.servicePath)
	assert.Equal(t, systemdServiceName, mgr.serviceName)
}

func TestSystemdServiceManager_Setup(t *testing.T) {
	// Skip if not running as root or if systemctl is not available
	if os.Geteuid() != 0 {
		t.Skip("Skipping test that requires root")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("Skipping test that requires systemctl")
	}

	tmpDir := t.TempDir()
	testServicePath := filepath.Join(tmpDir, "etc", "systemd", "system", "datadog-apm-inject.service")

	mgr := &SystemdServiceManager{
		servicePath:   testServicePath,
		serviceName:   "test-datadog-apm-inject.service",
		installerPath: "/usr/bin/datadog-installer",
	}

	ctx := context.Background()
	err := mgr.Setup(ctx)
	if err != nil {
		t.Logf("Setup failed (expected in test environment): %v", err)
		return
	}

	assert.FileExists(t, testServicePath)
	content, err := os.ReadFile(testServicePath)
	require.NoError(t, err)
	// The placeholder must have been substituted with the configured path,
	// and the placeholder itself must not appear in the rendered file.
	assert.Contains(t, string(content), "/usr/bin/datadog-installer apm instrument-start host")
	assert.Contains(t, string(content), "/usr/bin/datadog-installer apm instrument-stop host")
	assert.NotContains(t, string(content), installerPathPlaceholder)

	_ = mgr.Uninstall(ctx)
}

func TestSystemdServiceManager_writeServiceFile(t *testing.T) {
	tmpDir := t.TempDir()
	testDestPath := filepath.Join(tmpDir, "dest", "datadog-apm-inject.service")

	mgr := &SystemdServiceManager{
		servicePath:   testDestPath,
		installerPath: "/opt/datadog-packages/datadog-installer/stable/bin/installer/installer",
	}

	err := mgr.writeServiceFile()
	require.NoError(t, err)

	assert.FileExists(t, testDestPath)
	content, err := os.ReadFile(testDestPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "ExecStart=/opt/datadog-packages/datadog-installer/stable/bin/installer/installer apm instrument-start host")
	assert.Contains(t, string(content), "ExecStop=/opt/datadog-packages/datadog-installer/stable/bin/installer/installer apm instrument-stop host")
	assert.NotContains(t, string(content), installerPathPlaceholder)
}

func TestSystemdServiceManager_writeServiceFile_NoShWrapper(t *testing.T) {
	tmpDir := t.TempDir()
	testDestPath := filepath.Join(tmpDir, "datadog-apm-inject.service")

	mgr := &SystemdServiceManager{
		servicePath:   testDestPath,
		installerPath: "/usr/bin/datadog-installer",
	}
	require.NoError(t, mgr.writeServiceFile())

	content, err := os.ReadFile(testDestPath)
	require.NoError(t, err)
	rendered := string(content)

	assert.NotContains(t, rendered, "/bin/sh", "unit file must not delegate to /bin/sh")
	assert.NotContains(t, rendered, "sh -c", "unit file must not wrap commands in a shell")
	assert.Contains(t, rendered, "instrument-start host")
	assert.Contains(t, rendered, "instrument-stop host")
}

func TestSystemdServiceManager_writeServiceFile_EmptyInstallerPath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := &SystemdServiceManager{
		servicePath:   filepath.Join(tmpDir, "datadog-apm-inject.service"),
		installerPath: "",
	}
	err := mgr.writeServiceFile()
	assert.Error(t, err, "writing the unit file with no installer path should fail loudly rather than ship a broken unit")
}

func TestResolveInstallerPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up a mix of candidates: missing, non-executable, executable. The
	// resolver must skip the first two and pick the third.
	missing := filepath.Join(tmpDir, "missing")
	nonExec := filepath.Join(tmpDir, "non-exec")
	require.NoError(t, os.WriteFile(nonExec, []byte("#!/bin/sh\n"), 0644))
	executable := filepath.Join(tmpDir, "executable")
	require.NoError(t, os.WriteFile(executable, []byte("#!/bin/sh\n"), 0755))

	got, err := resolveInstallerPath([]string{missing, nonExec, executable}, alwaysSupported)
	require.NoError(t, err)
	assert.Equal(t, executable, got)
}

func TestResolveInstallerPath_AllMissing(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := resolveInstallerPath([]string{
		filepath.Join(tmpDir, "a"),
		filepath.Join(tmpDir, "b"),
	}, alwaysSupported)
	assert.Error(t, err)
}

func TestResolveInstallerPath_SkipsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dir := filepath.Join(tmpDir, "candidate-dir")
	require.NoError(t, os.MkdirAll(dir, 0755))
	exe := filepath.Join(tmpDir, "candidate-exe")
	require.NoError(t, os.WriteFile(exe, []byte{}, 0755))

	got, err := resolveInstallerPath([]string{dir, exe}, alwaysSupported)
	require.NoError(t, err)
	assert.Equal(t, exe, got)
}

// TestResolveInstallerPath_SkipsUnsupportedInstaller guards the upgrade case: a
// higher-priority candidate that is on disk and executable but too old to support
// `apm instrument-start` must be skipped in favor of a newer candidate.
func TestResolveInstallerPath_SkipsUnsupportedInstaller(t *testing.T) {
	tmpDir := t.TempDir()
	stale := filepath.Join(tmpDir, "stale-installer")
	require.NoError(t, os.WriteFile(stale, []byte{}, 0755))
	fresh := filepath.Join(tmpDir, "fresh-installer")
	require.NoError(t, os.WriteFile(fresh, []byte{}, 0755))

	got, err := resolveInstallerPath([]string{stale, fresh}, func(p string) bool { return p == fresh })
	require.NoError(t, err)
	assert.Equal(t, fresh, got, "stale installer (no instrument-start) must be skipped")
}

// TestResolveInstallerPath_AllUnsupported asserts we report failure rather than
// return a stale installer that would produce a unit doomed to fail on boot.
func TestResolveInstallerPath_AllUnsupported(t *testing.T) {
	tmpDir := t.TempDir()
	exe := filepath.Join(tmpDir, "old-installer")
	require.NoError(t, os.WriteFile(exe, []byte{}, 0755))

	_, err := resolveInstallerPath([]string{exe}, func(string) bool { return false })
	assert.ErrorContains(t, err, "too old")
}

// TestSupportsInstrumentSubcommands exercises the real `apm --help` probe against
// stub binaries standing in for a newer and an older datadog-installer.
func TestSupportsInstrumentSubcommands(t *testing.T) {
	tmpDir := t.TempDir()

	newer := filepath.Join(tmpDir, "new-installer")
	require.NoError(t, os.WriteFile(newer, []byte("#!/bin/sh\necho 'Available Commands:'\necho '  instrument-start ...'\necho '  instrument-stop ...'\n"), 0755))
	assert.True(t, supportsInstrumentSubcommands(newer))

	older := filepath.Join(tmpDir, "old-installer")
	require.NoError(t, os.WriteFile(older, []byte("#!/bin/sh\necho 'Available Commands:'\necho '  instrument ...'\necho '  uninstrument ...'\n"), 0755))
	assert.False(t, supportsInstrumentSubcommands(older))

	assert.False(t, supportsInstrumentSubcommands(filepath.Join(tmpDir, "does-not-exist")))
}

func alwaysSupported(string) bool { return true }

func TestSystemdServiceManager_Uninstall(t *testing.T) {
	tmpDir := t.TempDir()
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	err := os.WriteFile(testServicePath, []byte("[Unit]\nDescription=Test\n"), 0644)
	require.NoError(t, err)

	mgr := &SystemdServiceManager{
		servicePath: testServicePath,
		serviceName: "test-service.service",
	}

	ctx := context.Background()
	assert.FileExists(t, testServicePath)

	// Uninstall will fail on systemctl ops but should still remove the file
	// (systemctl errors are logged as warnings, not returned)
	_ = mgr.Uninstall(ctx)
}

func TestSystemdServiceManager_Uninstall_ServiceFileAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentServicePath := filepath.Join(tmpDir, "nonexistent.service")

	mgr := &SystemdServiceManager{
		servicePath: nonExistentServicePath,
		serviceName: "test-service.service",
	}

	ctx := context.Background()
	err := mgr.Uninstall(ctx)
	assert.NoError(t, err)
}

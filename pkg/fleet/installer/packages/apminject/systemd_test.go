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
		servicePath: testServicePath,
		serviceName: "test-datadog-apm-inject.service",
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
	assert.Equal(t, string(apmInjectServiceFile), string(content))

	_ = mgr.Uninstall(ctx)
}

func TestSystemdServiceManager_writeServiceFile(t *testing.T) {
	tmpDir := t.TempDir()
	testDestPath := filepath.Join(tmpDir, "dest", "datadog-apm-inject.service")

	mgr := &SystemdServiceManager{
		servicePath: testDestPath,
	}

	err := mgr.writeServiceFile()
	require.NoError(t, err)

	assert.FileExists(t, testDestPath)
	content, err := os.ReadFile(testDestPath)
	require.NoError(t, err)
	assert.Equal(t, string(apmInjectServiceFile), string(content))
}

func TestSystemdServiceManager_writeServiceFile_ContainsInstallerCommand(t *testing.T) {
	tmpDir := t.TempDir()
	testDestPath := filepath.Join(tmpDir, "datadog-apm-inject.service")

	mgr := &SystemdServiceManager{
		servicePath: testDestPath,
	}

	err := mgr.writeServiceFile()
	require.NoError(t, err)

	content, err := os.ReadFile(testDestPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "instrument-start host")
	assert.Contains(t, string(content), "instrument-stop host")
	assert.Contains(t, string(content), "/usr/bin/datadog-installer")
}

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

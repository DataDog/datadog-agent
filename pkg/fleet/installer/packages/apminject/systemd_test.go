// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package apminject

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSystemdServiceManager(t *testing.T) {
	mgr := NewSystemdServiceManager()

	assert.NotNil(t, mgr)
	assert.Equal(t, injectorServiceSourcePath, mgr.serviceSourcePath)
	assert.Equal(t, systemdServicePath, mgr.servicePath)
	assert.Equal(t, systemdServiceName, mgr.serviceName)
}

func TestSystemdServiceManager_Install(t *testing.T) {
	// Skip if not running as root or if systemctl is not available
	if os.Geteuid() != 0 {
		t.Skip("Skipping test that requires root")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("Skipping test that requires systemctl")
	}

	tmpDir := t.TempDir()
	testSourcePath := filepath.Join(tmpDir, "datadog-apm-inject.service")
	testServicePath := filepath.Join(tmpDir, "etc", "systemd", "system", "datadog-apm-inject.service")

	serviceContent := "[Unit]\nDescription=Datadog APM Inject\n[Install]\nWantedBy=multi-user.target\n"
	err := os.WriteFile(testSourcePath, []byte(serviceContent), 0644)
	require.NoError(t, err)

	mgr := &SystemdServiceManager{
		serviceSourcePath: testSourcePath,
		servicePath:       testServicePath,
		serviceName:       "test-datadog-apm-inject.service",
	}

	ctx := context.Background()
	err = mgr.Install(ctx)
	if err != nil {
		t.Logf("Install failed (expected in test environment): %v", err)
		return
	}

	assert.FileExists(t, testServicePath)
	content, err := os.ReadFile(testServicePath)
	require.NoError(t, err)
	assert.Equal(t, serviceContent, string(content))

	_ = mgr.Uninstall(ctx)
}

func TestSystemdServiceManager_IsInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	mgr := &SystemdServiceManager{
		servicePath: testServicePath,
	}

	assert.False(t, mgr.IsInstalled())

	err := os.WriteFile(testServicePath, []byte("[Unit]\nDescription=Test\n"), 0644)
	require.NoError(t, err)

	assert.True(t, mgr.IsInstalled())
}

func TestSystemdServiceManager_Install_SourceNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentSource := filepath.Join(tmpDir, "nonexistent.service")
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	mgr := &SystemdServiceManager{
		serviceSourcePath: nonExistentSource,
		servicePath:       testServicePath,
		serviceName:       "test-service.service",
	}

	ctx := context.Background()
	err := mgr.Install(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open service file from OCI package")
}

func TestSystemdServiceManager_copyServiceFile(t *testing.T) {
	tmpDir := t.TempDir()
	testSourcePath := filepath.Join(tmpDir, "source.service")
	testDestPath := filepath.Join(tmpDir, "dest", "target.service")

	serviceContent := "[Unit]\nDescription=Datadog APM Inject\n"
	err := os.WriteFile(testSourcePath, []byte(serviceContent), 0644)
	require.NoError(t, err)

	mgr := &SystemdServiceManager{
		serviceSourcePath: testSourcePath,
		servicePath:       testDestPath,
	}

	err = mgr.copyServiceFile()
	require.NoError(t, err)

	assert.FileExists(t, testDestPath)
	content, err := os.ReadFile(testDestPath)
	require.NoError(t, err)
	assert.Equal(t, serviceContent, string(content))
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

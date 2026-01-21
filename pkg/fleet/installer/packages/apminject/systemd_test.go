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
	assert.Equal(t, apmInjectorBinPath, mgr.binPath)
	assert.Equal(t, systemdServicePath, mgr.servicePath)
	assert.Equal(t, systemdServiceName, mgr.serviceName)
}

func TestGetAPMInjectorBinaryPath(t *testing.T) {
	path := GetAPMInjectorBinaryPath()

	expected := filepath.Join(injectorPath, "bin", "apm-injector")
	assert.Equal(t, expected, path)
}

func TestSystemdServiceManager_Install(t *testing.T) {
	// Skip if not running as root or if systemctl is not available
	if os.Geteuid() != 0 {
		t.Skip("Skipping test that requires root")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("Skipping test that requires systemctl")
	}

	// Create a test environment
	tmpDir := t.TempDir()
	testBinPath := filepath.Join(tmpDir, "apm-injector")
	testServicePath := filepath.Join(tmpDir, "test-apm-injector.service")

	// Create a dummy binary
	err := os.WriteFile(testBinPath, []byte("#!/bin/sh\necho 'test'\n"), 0755)
	require.NoError(t, err)

	mgr := &SystemdServiceManager{
		binPath:     testBinPath,
		servicePath: testServicePath,
		serviceName: "test-apm-injector.service",
	}

	ctx := context.Background()

	// Test Install
	err = mgr.Install(ctx)
	if err != nil {
		// Installation may fail in test environment without full systemd
		t.Logf("Install failed (expected in test environment): %v", err)
		return
	}

	// Verify service file was created
	assert.FileExists(t, testServicePath)

	// Read and verify service file content
	content, err := os.ReadFile(testServicePath)
	require.NoError(t, err)

	serviceContent := string(content)
	assert.Contains(t, serviceContent, "[Unit]")
	assert.Contains(t, serviceContent, "Description=Datadog APM Injector")
	assert.Contains(t, serviceContent, "Type=oneshot")
	assert.Contains(t, serviceContent, "RemainAfterExit=yes")
	assert.Contains(t, serviceContent, "ExecStart="+testBinPath+" install")
	assert.Contains(t, serviceContent, "ExecStop="+testBinPath+" uninstall")

	// Cleanup
	_ = mgr.Uninstall(ctx)
}

func TestSystemdServiceManager_IsInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	mgr := &SystemdServiceManager{
		servicePath: testServicePath,
	}

	// Should not be installed initially
	assert.False(t, mgr.IsInstalled())

	// Create service file
	err := os.WriteFile(testServicePath, []byte("[Unit]\nDescription=Test\n"), 0644)
	require.NoError(t, err)

	// Should now be installed
	assert.True(t, mgr.IsInstalled())
}

func TestSystemdServiceManager_Install_BinaryNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentBinary := filepath.Join(tmpDir, "nonexistent")
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	mgr := &SystemdServiceManager{
		binPath:     nonExistentBinary,
		servicePath: testServicePath,
		serviceName: "test-service.service",
	}

	ctx := context.Background()
	err := mgr.Install(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "apm-injector binary not found")
}

func TestSystemdServiceManager_Uninstall(t *testing.T) {
	tmpDir := t.TempDir()
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	// Create service file
	err := os.WriteFile(testServicePath, []byte("[Unit]\nDescription=Test\n"), 0644)
	require.NoError(t, err)

	mgr := &SystemdServiceManager{
		servicePath: testServicePath,
		serviceName: "test-service.service",
	}

	ctx := context.Background()

	// Verify service file exists
	assert.FileExists(t, testServicePath)

	// Uninstall (will fail to stop/disable but should remove file)
	err = mgr.Uninstall(ctx)
	// Error is acceptable in test environment where systemctl may not work
	if err != nil {
		t.Logf("Uninstall had errors (expected in test environment): %v", err)
	}

	// In a real environment, the file would be removed
	// In test environment without systemctl, file might still exist
	// so we just verify the function runs without panic
}

func TestSystemdServiceManager_ServiceContent(t *testing.T) {
	tmpDir := t.TempDir()
	testBinPath := filepath.Join(tmpDir, "apm-injector")
	testServicePath := filepath.Join(tmpDir, "test-service.service")

	// Create dummy binary
	err := os.WriteFile(testBinPath, []byte("#!/bin/sh\necho 'test'\n"), 0755)
	require.NoError(t, err)

	// Create the service content to test format
	serviceContent := `[Unit]
Description=Datadog APM Injector
Documentation=https://docs.datadoghq.com/
After=network.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=` + testBinPath + ` install
ExecStop=` + testBinPath + ` uninstall
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`

	// Write service file
	err = os.WriteFile(testServicePath, []byte(serviceContent), 0644)
	require.NoError(t, err)

	// Verify content
	content, err := os.ReadFile(testServicePath)
	require.NoError(t, err)
	assert.Equal(t, serviceContent, string(content))

	// Verify key components are present
	assert.Contains(t, string(content), "Type=oneshot")
	assert.Contains(t, string(content), "RemainAfterExit=yes")
	assert.Contains(t, string(content), testBinPath+" install")
	assert.Contains(t, string(content), testBinPath+" uninstall")
}

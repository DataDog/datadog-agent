// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/client"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/module"
	apimodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
)

// setupTestServer creates a test server with the privileged logs module and returns the socket path and temp directory
func setupTestServer(t *testing.T) (string, string, func()) {
	return setupTestServerWithModule(t, nil)
}

// setupTestServerWithModule creates a test server with the provided privileged logs module or creates a new one
// createTestFile creates a test file with specified content and permissions
func createTestFile(t *testing.T, dir, filename, content string, perm os.FileMode) string {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), perm)
	require.NoError(t, err)
	return filePath
}

// setupSystemProbeConfig creates a system-probe config mock with the given socket path and enabled status
func setupSystemProbeConfig(t *testing.T, socketPath string, enabled bool) {
	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", socketPath)
	systemProbeConfig.SetWithoutSource("privileged_logs.enabled", enabled)
}

// assertFileContentEquals opens a file via client and verifies its content matches expected
func assertFileContentEquals(t *testing.T, socketPath, filePath, expectedContent string) {
	file, err := client.OpenFile(socketPath, filePath)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(data))
}

// assertClientOpenError verifies that client.Open returns an error containing the expected message
func assertClientOpenError(t *testing.T, filePath, expectedErrorMsg string) {
	file, err := client.Open(filePath)
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Contains(t, err.Error(), expectedErrorMsg)
}

// assertClientOpenFileError verifies that client.OpenFile returns an error containing the expected message
func assertClientOpenFileError(t *testing.T, socketPath, filePath, expectedErrorMsg string) {
	file, err := client.OpenFile(socketPath, filePath)
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Contains(t, err.Error(), expectedErrorMsg)
}

// createModuleAndServer creates a new privileged logs module and test server
func createModuleAndServer(t *testing.T) (apimodule.Module, string, string, func()) {
	fdModule := module.NewPrivilegedLogsModule()
	require.NotNil(t, fdModule)
	socketPath, tempDir, cleanup := setupTestServerWithModule(t, fdModule)
	return fdModule, socketPath, tempDir, cleanup
}

func setupTestServerWithModule(t *testing.T, existingModule apimodule.Module) (string, string, func()) {
	var fdModule apimodule.Module

	if existingModule != nil {
		fdModule = existingModule
	} else {
		// Create the privileged logs module directly
		fdModule = module.NewPrivilegedLogsModule()
		require.NotNil(t, fdModule)
	}

	// Create a test server with the module
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")
	listener, err := server.NewListener(socketPath)
	require.NoError(t, err)

	// Set up HTTP router and register the module
	httpMux := mux.NewRouter()
	router := apimodule.NewRouter("privileged_logs", httpMux)
	err = fdModule.Register(router)
	require.NoError(t, err)

	// Create HTTP server
	httpServer := &http.Server{
		Handler: httpMux,
	}

	// Start the server
	go func() {
		httpServer.Serve(listener)
	}()

	// Give the server a moment to start up
	time.Sleep(30 * time.Millisecond)

	// Return cleanup function
	cleanup := func() {
		httpServer.Close()
		listener.Close()
	}

	return socketPath, tempDir, cleanup
}

func TestPrivilegedLogsModule(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Hello, privileged logs transfer!"
	testFile := createTestFile(t, tempDir, "test.log", testContent, 0644)

	_, socketPath, _, cleanup := createModuleAndServer(t)
	defer cleanup()

	assertFileContentEquals(t, socketPath, testFile, testContent)
}

func TestPrivilegedLogsModule_FileNotFound(t *testing.T) {
	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	assertClientOpenFileError(t, socketPath, "/nonexistent/file.log", "failed to resolve path")
}

func TestPrivilegedLogsModule_RelativePath(t *testing.T) {
	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	assertClientOpenFileError(t, socketPath, "relative/path.log", "relative path not allowed")
}

func TestPrivilegedLogsModule_NonLogFile(t *testing.T) {
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	nonLogFile := createTestFile(t, tempDir, "test.txt", "test content", 0644)
	assertClientOpenFileError(t, socketPath, nonLogFile, "non-log file not allowed")
}

func TestPrivilegedLogsModule_SymlinkProtection(t *testing.T) {
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	testContent := "real log content"
	realLogFile := createTestFile(t, tempDir, "real.log", testContent, 0644)

	symlinkPath := filepath.Join(tempDir, "fake.log")
	err := os.Symlink(realLogFile, symlinkPath)
	require.NoError(t, err)

	assertFileContentEquals(t, socketPath, symlinkPath, testContent)
}

func TestPrivilegedLogsModule_SymlinkToNonLogFile(t *testing.T) {
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	nonLogFile := createTestFile(t, tempDir, "secret.txt", "secret content", 0644)

	symlinkPath := filepath.Join(tempDir, "fake.log")
	err := os.Symlink(nonLogFile, symlinkPath)
	require.NoError(t, err)

	assertClientOpenFileError(t, socketPath, symlinkPath, "non-log file not allowed")
}

func TestPrivilegedLogsModule_CaseInsensitiveLogExtension(t *testing.T) {
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	testContent := "test content"
	upperLogFile := createTestFile(t, tempDir, "test.LOG", testContent, 0644)
	mixedLogFile := createTestFile(t, tempDir, "test.Log", testContent, 0644)

	assertFileContentEquals(t, socketPath, upperLogFile, testContent)
	assertFileContentEquals(t, socketPath, mixedLogFile, testContent)
}

func TestPrivilegedLogsModule_Close(t *testing.T) {
	// Create the privileged logs module directly
	fdModule := module.NewPrivilegedLogsModule()
	require.NotNil(t, fdModule)

	// Test that Close doesn't panic
	fdModule.Close()
}

func TestOpen_SuccessfulNormalOpen(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Hello, privileged logs transfer!"
	testFile := createTestFile(t, tempDir, "test.log", testContent, 0644)

	file, err := client.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(data))
}

func TestOpen_WithModuleEnabled(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Test content"
	testFile := createTestFile(t, tempDir, "test.log", testContent, 0644)

	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	setupSystemProbeConfig(t, socketPath, true)

	file, err := client.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(data))
}

func TestOpen_PermissionErrorWithModuleDisabled(t *testing.T) {
	tempDir := t.TempDir()
	testFile := createTestFile(t, tempDir, "restricted.log", "Restricted content", 0000)

	setupSystemProbeConfig(t, "/nonexistent/socket", false)

	assertClientOpenError(t, testFile, "permission denied")
}

func TestOpen_PermissionErrorWithModuleFailure(t *testing.T) {
	tempDir := t.TempDir()
	testFile := createTestFile(t, tempDir, "restricted.log", "Restricted content", 0000)

	setupSystemProbeConfig(t, "/nonexistent/socket", true)

	file, err := client.Open(testFile)
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Contains(t, err.Error(), "failed to open file with system-probe")
	assert.Contains(t, err.Error(), "permission denied")
}

func TestOpen_NonPermissionError(t *testing.T) {
	file, err := client.Open("/nonexistent/file.log")
	require.Error(t, err)
	assert.Nil(t, file)
	assert.NotContains(t, err.Error(), "failed to open file with system-probe")
	assert.NotContains(t, err.Error(), "system-probe")
}

func TestOpen_NonLogFile(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Test content"
	testFile := createTestFile(t, tempDir, "test.txt", testContent, 0644)

	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	setupSystemProbeConfig(t, socketPath, true)

	file, err := client.Open(testFile)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(data))
}

func TestOpen_RelativePath(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Test content"
	createTestFile(t, tempDir, "test.log", testContent, 0644)

	originalWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalWd)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	setupSystemProbeConfig(t, socketPath, true)

	file, err := client.Open("test.log")
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(data))
}

func TestOpen_Symlink(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Real log content"
	realLogFile := createTestFile(t, tempDir, "real.log", testContent, 0644)

	symlinkPath := filepath.Join(tempDir, "fake.log")
	err := os.Symlink(realLogFile, symlinkPath)
	require.NoError(t, err)

	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	setupSystemProbeConfig(t, socketPath, true)

	file, err := client.Open(symlinkPath)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(data))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

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

	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// setupTestServer creates a test server with the FD transfer module and returns the socket path and temp directory
func setupTestServer(t *testing.T) (string, string, func()) {
	return setupTestServerWithModule(t, nil)
}

// setupTestServerWithModule creates a test server with the provided FD transfer module or creates a new one
func setupTestServerWithModule(t *testing.T, existingModule module.Module) (string, string, func()) {
	var fdModule module.Module
	var err error

	if existingModule != nil {
		fdModule = existingModule
	} else {
		// Create the fd transfer module
		cfg := &sysconfigtypes.Config{}
		deps := module.FactoryDependencies{}

		fdModule, err = modules.FDTransfer.Fn(cfg, deps)
		require.NoError(t, err)
		require.NotNil(t, fdModule)
	}

	// Create a test server with the module
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")
	listener, err := server.NewListener(socketPath)
	require.NoError(t, err)

	// Set up HTTP router and register the module
	httpMux := mux.NewRouter()
	router := module.NewRouter("fd_transfer", httpMux)
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

func TestFDTransferModule(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.log")
	testContent := "Hello, file descriptor transfer!"

	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, tempDir, cleanup := setupTestServerWithModule(t, fdModule)
	defer cleanup()

	// Test the file descriptor transfer using the client
	file, err := client.OpenFile(socketPath, testFile)
	require.NoError(t, err)
	defer file.Close()

	// Verify we can read from the transferred file descriptor
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, testContent, string(data))

	// Test stats
	stats := fdModule.GetStats()
	assert.Contains(t, stats, "last_check")
	assert.Greater(t, stats["last_check"].(int64), int64(0))
}

func TestFDTransferModule_FileNotFound(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Test with non-existent file
	_, err = client.OpenFile(socketPath, "/nonexistent/file.log")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Failed to resolve path")
}

func TestFDTransferModule_RelativePath(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Test with relative path (should be rejected)
	_, err = client.OpenFile(socketPath, "relative/path.log")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Only absolute paths are allowed")
}

func TestFDTransferModule_NonLogFile(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a non-log file
	nonLogFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(nonLogFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Test with non-log file (should be rejected)
	_, err = client.OpenFile(socketPath, nonLogFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Only log files (ending with .log) are allowed")
}

func TestFDTransferModule_SymlinkProtection(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a real log file
	realLogFile := filepath.Join(tempDir, "real.log")
	err = os.WriteFile(realLogFile, []byte("real log content"), 0644)
	require.NoError(t, err)

	// Create a symlink pointing to the real log file
	symlinkPath := filepath.Join(tempDir, "fake.log")
	err = os.Symlink(realLogFile, symlinkPath)
	require.NoError(t, err)

	// Test with symlink to log file (should work now)
	file, err := client.OpenFile(socketPath, symlinkPath)
	require.NoError(t, err)
	defer file.Close()

	// Verify we can read from the transferred file descriptor
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, "real log content", string(data))
}

func TestFDTransferModule_SymlinkToNonLogFile(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a non-log file
	nonLogFile := filepath.Join(tempDir, "secret.txt")
	err = os.WriteFile(nonLogFile, []byte("secret content"), 0644)
	require.NoError(t, err)

	// Create a symlink pointing to the non-log file
	symlinkPath := filepath.Join(tempDir, "fake.log")
	err = os.Symlink(nonLogFile, symlinkPath)
	require.NoError(t, err)

	// Test with symlink to non-log file (should be rejected)
	_, err = client.OpenFile(socketPath, symlinkPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Only log files (ending with .log) are allowed")
}

func TestFDTransferModule_CaseInsensitiveLogExtension(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Create a test server with the module
	socketPath, tempDir, cleanup := setupTestServer(t)
	defer cleanup()

	// Test with uppercase .LOG extension
	upperLogFile := filepath.Join(tempDir, "test.LOG")
	err = os.WriteFile(upperLogFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Should work with uppercase .LOG
	file, err := client.OpenFile(socketPath, upperLogFile)
	require.NoError(t, err)
	defer file.Close()

	// Test with mixed case .Log extension
	mixedLogFile := filepath.Join(tempDir, "test.Log")
	err = os.WriteFile(mixedLogFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Should work with mixed case .Log
	file2, err := client.OpenFile(socketPath, mixedLogFile)
	require.NoError(t, err)
	defer file2.Close()
}

func TestFDTransferModule_Close(t *testing.T) {
	// Create the fd transfer module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.FDTransfer.Fn(cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, fdModule)

	// Test that Close doesn't panic
	fdModule.Close()
}

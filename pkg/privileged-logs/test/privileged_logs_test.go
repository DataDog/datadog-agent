// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/client"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/module"
	apimodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
)

func createTestFile(t *testing.T, dir, filename, content string, perm os.FileMode) string {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), perm)
	require.NoError(t, err)
	return filePath
}

func setupSystemProbeConfig(t *testing.T, socketPath string, enabled bool) {
	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", socketPath)
	systemProbeConfig.SetWithoutSource("privileged_logs.enabled", enabled)
}

func assertOpenPrivilegedContent(t *testing.T, socketPath, filePath, expectedContent string) {
	file, err := client.OpenPrivileged(socketPath, filePath)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(data))
}

func assertClientOpenError(t *testing.T, filePath, expectedErrorMsg string) {
	file, err := client.Open(filePath)
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Contains(t, err.Error(), expectedErrorMsg)
}

func assertClientOpenContent(t *testing.T, filePath, expectedContent string) {
	file, err := client.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	data, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(data))
}

func assertOpenPrivilegedError(t *testing.T, socketPath, filePath, expectedErrorMsg string) {
	file, err := client.OpenPrivileged(socketPath, filePath)
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Contains(t, err.Error(), expectedErrorMsg)
}

func setupTestServer(t *testing.T) (string, string, func()) {
	var fdModule apimodule.Module

	fdModule = module.NewPrivilegedLogsModule()
	require.NotNil(t, fdModule)

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

	httpServer := &http.Server{
		Handler: httpMux,
	}

	go func() {
		httpServer.Serve(listener)
	}()

	// Give the server a moment to start up
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		conn, err := net.Dial("unix", socketPath)
		require.NoError(collect, err)
		conn.Close()
	}, 1*time.Second, 10*time.Millisecond)

	cleanup := func() {
		httpServer.Shutdown(context.Background())
		listener.Close()
	}

	return socketPath, tempDir, cleanup
}

type PrivilegedLogsSuite struct {
	suite.Suite
	socketPath string
	tempDir    string
	cleanup    func()
}

func (s *PrivilegedLogsSuite) SetupSuite() {
	s.socketPath, s.tempDir, s.cleanup = setupTestServer(s.T())
}

func (s *PrivilegedLogsSuite) TearDownSuite() {
	s.cleanup()
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule() {
	testContent := "Hello, privileged logs transfer!"
	testFile := createTestFile(s.T(), s.tempDir, "test_main.log", testContent, 0644)

	assertOpenPrivilegedContent(s.T(), s.socketPath, testFile, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_FileNotFound() {
	assertOpenPrivilegedError(s.T(), s.socketPath, "/nonexistent/file.log", "failed to resolve path")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_RelativePath() {
	assertOpenPrivilegedError(s.T(), s.socketPath, "relative/path.log", "relative path not allowed")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_NonLogFile() {
	nonLogFile := createTestFile(s.T(), s.tempDir, "test_nonlog.txt", "test content", 0644)
	assertOpenPrivilegedError(s.T(), s.socketPath, nonLogFile, "non-log file not allowed")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_Symlink() {
	testContent := "real log content"
	realLogFile := createTestFile(s.T(), s.tempDir, "real.log", testContent, 0644)

	symlinkPath := filepath.Join(s.tempDir, "fake.log")
	err := os.Symlink(realLogFile, symlinkPath)
	require.NoError(s.T(), err)

	assertOpenPrivilegedContent(s.T(), s.socketPath, symlinkPath, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_SymlinkToNonLogFile() {
	nonLogFile := createTestFile(s.T(), s.tempDir, "secret_nonlog.txt", "secret content", 0644)

	symlinkPath := filepath.Join(s.tempDir, "fake_nonlog.log")
	err := os.Symlink(nonLogFile, symlinkPath)
	require.NoError(s.T(), err)

	assertOpenPrivilegedError(s.T(), s.socketPath, symlinkPath, "non-log file not allowed")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_CaseInsensitiveLogExtension() {
	testContent := "test content"
	upperLogFile := createTestFile(s.T(), s.tempDir, "test_upper.LOG", testContent, 0644)
	mixedLogFile := createTestFile(s.T(), s.tempDir, "test_mixed.Log", testContent, 0644)

	assertOpenPrivilegedContent(s.T(), s.socketPath, upperLogFile, testContent)
	assertOpenPrivilegedContent(s.T(), s.socketPath, mixedLogFile, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_OpenFallback() {
	testContent := "test content"
	upperLogFile := createTestFile(s.T(), s.tempDir, "restricted.log", testContent, 0000)

	setupSystemProbeConfig(s.T(), s.socketPath, true)

	// The module will also fail in this case (since the module has the same
	// permissions as the test code) but ensure that it has been attempted.  In
	// this test suite we do not check for the module succeeding while the
	// normal open failing, there is a test for that in
	// pkg/logs/launcher/file/launcher_privileged_logs_test.go.
	assertClientOpenError(s.T(), upperLogFile, "permission denied, original error")
}

func TestPrivilegedLogsSuite(t *testing.T) {
	suite.Run(t, new(PrivilegedLogsSuite))
}

func TestPrivilegedLogsModule_Close(t *testing.T) {
	fdModule := module.NewPrivilegedLogsModule()
	require.NotNil(t, fdModule)

	fdModule.Close()
}

func TestOpen_SuccessfulNormalOpen(t *testing.T) {
	tempDir := t.TempDir()
	testContent := "Hello, privileged logs transfer!"
	testFile := createTestFile(t, tempDir, "test.log", testContent, 0644)

	assertClientOpenContent(t, testFile, testContent)
}

func TestOpen_PermissionErrorWithModuleDisabled(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Cannot test permission error when running as root")
	}

	tempDir := t.TempDir()
	testFile := createTestFile(t, tempDir, "restricted.log", "Restricted content", 0000)

	setupSystemProbeConfig(t, "/nonexistent/socket", false)

	assertClientOpenError(t, testFile, "permission denied")
}

func TestOpen_PermissionErrorWithModuleFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("Cannot test permission error when running as root")
	}

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
	assert.NotContains(t, err.Error(), "system-probe")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

// Package test provides tests for the privileged logs module.
package test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/client"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/module"
)

func createTestFile(t *testing.T, dir, filename, content string) string {
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0000)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove(filePath)
	})
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

type PrivilegedLogsSuite struct {
	suite.Suite
	handler *Handler
	tempDir string
}

func (s *PrivilegedLogsSuite) SetupSuite() {
	s.handler = Setup(s.T(), func() {
		s.tempDir = s.T().TempDir()
	})
}

func (s *PrivilegedLogsSuite) TearDownSuite() {
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule() {
	testContent := "Hello, privileged logs transfer!"
	testFile := createTestFile(s.T(), s.tempDir, "test_main.log", testContent)

	assertOpenPrivilegedContent(s.T(), s.handler.SocketPath, testFile, testContent)
	assertOpenPrivilegedContent(s.T(), s.handler.SocketPath, testFile, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_FileNotFound() {
	assertOpenPrivilegedError(s.T(), s.handler.SocketPath, "/nonexistent/file.log", "failed to resolve path")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_RelativePath() {
	assertOpenPrivilegedError(s.T(), s.handler.SocketPath, "relative/path.log", "relative path not allowed")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_NonLogFile() {
	nonLogFile := createTestFile(s.T(), s.tempDir, "test_nonlog.txt", "test content")
	assertOpenPrivilegedError(s.T(), s.handler.SocketPath, nonLogFile, "non-log file not allowed")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_Symlink() {
	testContent := "real log content"
	realLogFile := createTestFile(s.T(), s.tempDir, "real.log", testContent)

	symlinkPath := filepath.Join(s.tempDir, "fake.log")
	err := os.Symlink(realLogFile, symlinkPath)
	require.NoError(s.T(), err)

	assertOpenPrivilegedContent(s.T(), s.handler.SocketPath, symlinkPath, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_SymlinkToNonLogFile() {
	nonLogFile := createTestFile(s.T(), s.tempDir, "secret_nonlog.txt", "secret content")

	symlinkPath := filepath.Join(s.tempDir, "fake_nonlog.log")
	err := os.Symlink(nonLogFile, symlinkPath)
	require.NoError(s.T(), err)

	assertOpenPrivilegedError(s.T(), s.handler.SocketPath, symlinkPath, "non-log file not allowed")
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_CaseInsensitiveLogExtension() {
	testContent := "test content"
	upperLogFile := createTestFile(s.T(), s.tempDir, "test_upper.LOG", testContent)
	mixedLogFile := createTestFile(s.T(), s.tempDir, "test_mixed.Log", testContent)

	assertOpenPrivilegedContent(s.T(), s.handler.SocketPath, upperLogFile, testContent)
	assertOpenPrivilegedContent(s.T(), s.handler.SocketPath, mixedLogFile, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_OpenFallback() {
	testContent := "test content"
	upperLogFile := createTestFile(s.T(), s.tempDir, "restricted.log", testContent)

	setupSystemProbeConfig(s.T(), s.handler.SocketPath, true)

	assertClientOpenContent(s.T(), upperLogFile, testContent)
}

func (s *PrivilegedLogsSuite) TestPrivilegedLogsModule_OpenFallbackError() {
	testContent := "test content"
	upperLogFile := createTestFile(s.T(), s.tempDir, "restricted.txt", testContent)

	setupSystemProbeConfig(s.T(), s.handler.SocketPath, true)

	assertClientOpenError(s.T(), upperLogFile, "non-log file not allowed")
}

func TestPrivilegedLogsSuite(t *testing.T) {
	suite.Run(t, new(PrivilegedLogsSuite))
}

func TestPrivilegedLogsModule_Close(t *testing.T) {
	fdModule := module.NewPrivilegedLogsModule()
	require.NotNil(t, fdModule)

	fdModule.Close()
}

func (s *PrivilegedLogsSuite) TestOpen_SuccessfulNormalOpen() {
	testContent := "Hello, privileged logs transfer!"
	testFile := createTestFile(s.T(), s.tempDir, "test.log", testContent)

	// Force the file to be accesssible from the non-privileged user
	require.NoError(s.T(), os.Chmod(testFile, 0644))

	assertClientOpenContent(s.T(), testFile, testContent)
}

func (s *PrivilegedLogsSuite) TestOpen_PermissionErrorWithModuleDisabled() {
	testFile := createTestFile(s.T(), s.tempDir, "restricted.log", "Restricted content")

	setupSystemProbeConfig(s.T(), s.handler.SocketPath, false)

	assertClientOpenError(s.T(), testFile, "permission denied")
}

func (s *PrivilegedLogsSuite) TestOpen_PermissionErrorWithModuleFailure() {
	t := s.T()
	testFile := createTestFile(s.T(), s.tempDir, "restricted.log", "Restricted content")

	setupSystemProbeConfig(s.T(), "/nonexistent/socket", true)

	file, err := client.Open(testFile)
	require.Error(t, err)
	assert.Nil(t, file)
	assert.Contains(t, err.Error(), "failed to open file with system-probe")
	assert.Contains(t, err.Error(), "permission denied")
}

func (s *PrivilegedLogsSuite) TestOpen_NonPermissionError() {
	file, err := client.Open("/nonexistent/file.log")
	t := s.T()
	require.Error(t, err)
	assert.Nil(t, file)
	assert.NotContains(t, err.Error(), "system-probe")
}

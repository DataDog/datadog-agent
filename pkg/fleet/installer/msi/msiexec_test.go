// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// mockCmdRunner for testing using testify/mock
type mockCmdRunner struct {
	mock.Mock
}

// Run executes the mock function
func (m *mockCmdRunner) Run(path string, cmdLine string) error {
	args := m.Called(path, cmdLine)
	return args.Error(0)
}

// mockExitError simulates exit errors for testing and implements ExitCodeError
type mockExitError struct {
	code int
}

func (m *mockExitError) Error() string {
	return fmt.Sprintf("exit status %d", m.code)
}

func (m *mockExitError) ExitCode() int {
	return m.code
}

// Verify mockExitError implements exitCodeError
var _ exitCodeError = (*mockExitError)(nil)

// Test isRetryableExitCode function and the exitCodeError interface
func TestIsRetryableExitCode(t *testing.T) {
	mockExitCodeTests := []struct {
		name        string
		err         error
		isRetryable bool
	}{
		{
			name:        "nil error",
			err:         nil,
			isRetryable: false,
		},
		{
			name:        "retryable exit code 1618",
			err:         &mockExitError{code: int(windows.ERROR_INSTALL_ALREADY_RUNNING)},
			isRetryable: true,
		},
		{
			name:        "non-retryable exit code 1603",
			err:         &mockExitError{code: 1603},
			isRetryable: false,
		},
		{
			name:        "non-retryable exit code 1",
			err:         &mockExitError{code: 1},
			isRetryable: false,
		},
		{
			name:        "generic error",
			err:         errors.New("generic error"),
			isRetryable: false,
		},
	}

	for _, tt := range mockExitCodeTests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableExitCode(tt.err)
			assert.Equal(t, tt.isRetryable, result, "Test case: %s", tt.name)
		})
	}

	parentT := t
	t.Run("real exit code tests", func(t *testing.T) {
		if parentT.Failed() {
			t.Skip("Skipping real exit code tests because mock tests failed")
		}

		realExitCodeTests := []struct {
			name        string
			exitCode    int
			isRetryable bool
		}{
			{
				name:        "retryable exit code 1618 (ERROR_INSTALL_ALREADY_RUNNING)",
				exitCode:    int(windows.ERROR_INSTALL_ALREADY_RUNNING), // 1618
				isRetryable: true,
			},
			{
				name:        "non-retryable exit code 1603",
				exitCode:    1603,
				isRetryable: false,
			},
			{
				name:        "non-retryable exit code 1",
				exitCode:    1,
				isRetryable: false,
			},
			{
				name:        "non-retryable exit code 0 (success)",
				exitCode:    0,
				isRetryable: false, // Success shouldn't be retried
			},
		}

		for _, tt := range realExitCodeTests {
			t.Run(tt.name, func(t *testing.T) {
				// Use cmd /c exit $EXITCODE to generate a real exec.ExitError
				cmdPath := filepath.Join(system32Path, "cmd.exe")
				runner := newRealCmdRunner()
				err := runner.Run(cmdPath, fmt.Sprintf("%s /c exit %d", cmdPath, tt.exitCode))

				if tt.exitCode == 0 {
					// Exit code 0 should not produce an error
					assert.NoError(t, err)
					result := isRetryableExitCode(err)
					assert.Equal(t, tt.isRetryable, result)
				} else {
					// Non-zero exit codes should produce exec.ExitError
					assert.Error(t, err)

					// Verify it's an exec.ExitError
					var exitError *exec.ExitError
					assert.ErrorAs(t, err, &exitError, "Error should be exec.ExitError")
					assert.Equal(t, tt.exitCode, exitError.ExitCode())

					// Verify exec.ExitError implements our ExitCodeError interface
					var interfaceError exitCodeError
					assert.ErrorAs(t, err, &interfaceError, "exec.ExitError should implement ExitCodeError")
					assert.Equal(t, tt.exitCode, interfaceError.ExitCode())

					// Test our retry logic with the real error
					result := isRetryableExitCode(err)
					assert.Equal(t, tt.isRetryable, result, "isRetryableExitCode should correctly identify retryable codes")
				}
			})
		}
	})
}

// Test Msiexec.Run retry behavior with retryable error followed by success
func TestMsiexec_Run_RetryThenSuccess(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	// First call fails with retryable error, second succeeds
	mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(&mockExitError{code: int(windows.ERROR_INSTALL_ALREADY_RUNNING)}).Once()
	mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()

	cmd, err := Cmd(
		Install(),
		WithMsi("test.msi"),
		WithLogFile("test.log"),
		withCmdRunner(mockRunner),
		// retry immediately for fast testing
		withBackOff(&backoff.ZeroBackOff{}),
	)
	require.NoError(t, err)

	_, err = cmd.Run(t.Context())
	assert.NoError(t, err)

	// Verify mock was called the expected number of times
	mockRunner.AssertExpectations(t)
}

// Test Msiexec.Run non-retryable error
func TestMsiexec_Run_NonRetryableError(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(&mockExitError{code: 1603}).Once()

	cmd, err := Cmd(
		Install(),
		WithMsi("test.msi"),
		WithLogFile("test.log"),
		withCmdRunner(mockRunner),
	)
	require.NoError(t, err)

	_, err = cmd.Run(t.Context())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1603")

	// Verify mock was called only once (no retry)
	mockRunner.AssertExpectations(t)
}

// Test command line construction
func TestMsiexec_CommandLineConstruction(t *testing.T) {
	t.Run("install with args", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}

		expectedCmdLine := fmt.Sprintf(`"%s" /i "test.msi" /qn /log "test.log" ARG1=value1 ARG2=value2 DDAGENTUSER_NAME=ddagent DDAGENTUSER_PASSWORD=password MSIFASTINSTALL=7`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			Install(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			WithDdAgentUserName("ddagent"),
			WithDdAgentUserPassword("password"),
			WithAdditionalArgs([]string{"ARG1=value1", "ARG2=value2"}),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		_, err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})

	t.Run("uninstall with args", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}
		expectedCmdLine := fmt.Sprintf(`"%s" /x "test.msi" /qn /log "test.log"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			Uninstall(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		_, err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})

	t.Run("admin install", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}
		expectedCmdLine := fmt.Sprintf(`"%s" /a "test.msi" /qn /log "test.log"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			AdministrativeInstall(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		_, err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})
}

// Test missing required arguments
func TestCmd_MissingRequiredArgs(t *testing.T) {
	tests := []struct {
		name    string
		options []MsiexecOption
	}{
		{
			name:    "missing action",
			options: []MsiexecOption{WithMsi("test.msi")},
		},
		{
			name:    "missing target",
			options: []MsiexecOption{Install()},
		},
		{
			name:    "no options",
			options: []MsiexecOption{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := Cmd(tt.options...)
			assert.Error(t, err)
			assert.Nil(t, cmd)
		})
	}
}

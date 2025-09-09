// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package msi

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding/unicode"
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
			name:        "retryable exit code 1601",
			err:         &mockExitError{code: int(windows.ERROR_INSTALL_SERVICE_FAILURE)},
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
				name:        "retryable exit code 1601 (ERROR_INSTALL_SERVICE_FAILURE)",
				exitCode:    int(windows.ERROR_INSTALL_SERVICE_FAILURE), // 1601
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
				err := runCmdWithExitCode(t, tt.exitCode)

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

	err = cmd.Run(t.Context())
	assert.NoError(t, err)

	// Verify mock was called the expected number of times
	mockRunner.AssertExpectations(t)
}

// Test isSuccessExitCode function and success exit code handling
func TestIsSuccessExitCode(t *testing.T) {
	mockExitCodeTests := []struct {
		name      string
		err       error
		expectsOk bool
	}{
		{
			name:      "nil error is success",
			err:       nil,
			expectsOk: true,
		},
		{
			name:      "success reboot required (3010)",
			err:       &mockExitError{code: int(windows.ERROR_SUCCESS_REBOOT_REQUIRED)},
			expectsOk: true,
		},
		{
			name:      "success reboot initiated (1641)",
			err:       &mockExitError{code: int(windows.ERROR_SUCCESS_REBOOT_INITIATED)},
			expectsOk: true,
		},
		{
			name:      "non-success exit code",
			err:       &mockExitError{code: 1603},
			expectsOk: false,
		},
		{
			name:      "generic error",
			err:       errors.New("some error"),
			expectsOk: false,
		},
	}

	for _, tt := range mockExitCodeTests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSuccessExitCode(tt.err)
			assert.Equal(t, tt.expectsOk, result)
		})
	}

	parentT := t
	t.Run("real exit code tests", func(t *testing.T) {
		if parentT.Failed() {
			t.Skip("Skipping real exit code tests because mock tests failed")
		}

		realExitCodeTests := []struct {
			name      string
			exitCode  int
			expectsOk bool
		}{
			{
				name:      "exit code 0 (success)",
				exitCode:  0,
				expectsOk: true,
			},
			{
				name:      "exit code 3010 (ERROR_SUCCESS_REBOOT_REQUIRED)",
				exitCode:  int(windows.ERROR_SUCCESS_REBOOT_REQUIRED),
				expectsOk: true,
			},
			{
				name:      "exit code 1641 (ERROR_SUCCESS_REBOOT_INITIATED)",
				exitCode:  int(windows.ERROR_SUCCESS_REBOOT_INITIATED),
				expectsOk: true,
			},
			{
				name:      "exit code 1603 (failure)",
				exitCode:  1603,
				expectsOk: false,
			},
		}

		for _, tt := range realExitCodeTests {
			t.Run(tt.name, func(t *testing.T) {
				err := runCmdWithExitCode(t, tt.exitCode)

				if tt.exitCode == 0 {
					// Exit code 0 should not produce an error
					assert.NoError(t, err)
					result := isSuccessExitCode(err)
					assert.Equal(t, tt.expectsOk, result)
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

					// Test our success logic with the real error
					result := isSuccessExitCode(err)
					assert.Equal(t, tt.expectsOk, result)
				}
			})
		}
	})
}

// Test Msiexec.Run treats success exit codes as success (no error returned)
func TestMsiexec_Run_SuccessExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
	}{
		{
			name:     "reboot required (3010)",
			exitCode: int(windows.ERROR_SUCCESS_REBOOT_REQUIRED),
		},
		{
			name:     "reboot initiated (1641)",
			exitCode: int(windows.ERROR_SUCCESS_REBOOT_INITIATED),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRunner := &mockCmdRunner{}
			// Only call Run once, we expect success exit codes to be handled outside of the retry loop
			mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(&mockExitError{code: tt.exitCode}).Once()

			cmd, err := Cmd(
				Install(),
				WithMsi("test.msi"),
				WithLogFile("test.log"),
				withCmdRunner(mockRunner),
			)
			require.NoError(t, err)

			err = cmd.Run(t.Context())
			assert.NoError(t, err, "success exit codes should not produce an error")
			mockRunner.AssertExpectations(t)
		})
	}
}

// TestMsiexec.Run retry behavior when MSI returns a non-retryable exit code, but the log file contains a retryable error
func TestMsiexec_Run_RetryableErrorInLog(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	// return a non-retryable exit code, so that the log file is searched for retryable error messages
	mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(&mockExitError{code: 1603}).Once()
	mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil).Once()
	// log file with retryable error
	logFile := createTestLogFile(t, "retryable-error.log", []byte(`
		Action start 17:48:55: WixSharp_InitRuntime_Action.
		CustomAction WixSharp_InitRuntime_Action returned actual error code 1601 (note this may not be 100% accurate if translation happened inside sandbox)
		MSI (s) (E4:18) [17:48:56:009]: Product: Datadog Agent -- Error 1719. The Windows Installer Service could not be accessed. This can occur if you are running Windows in safe mode, or if the Windows Installer is not correctly installed. Contact your support personnel for assistance.
	`))

	cmd, err := Cmd(
		Install(),
		WithMsi("test.msi"),
		WithLogFile(logFile),
		withCmdRunner(mockRunner),
		// retry immediately for fast testing
		withBackOff(&backoff.ZeroBackOff{}),
	)
	require.NoError(t, err)

	err = cmd.Run(t.Context())
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

	err = cmd.Run(t.Context())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exit status 1603")

	// Verify mock was called only once (no retry)
	mockRunner.AssertExpectations(t)
}

// Test command line construction
func TestMsiexec_CommandLineConstruction(t *testing.T) {
	t.Run("install with args", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}

		expectedCmdLine := fmt.Sprintf(`"%s" /i "test.msi" /qn /norestart /log "test.log" ARG1=value1 ARG2="value2" DDAGENTUSER_NAME="ddagent" DDAGENTUSER_PASSWORD="password" MSIFASTINSTALL="7"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			Install(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			WithDdAgentUserName("ddagent"),
			WithDdAgentUserPassword("password"),
			// Expect WithAdditionalArgs to be verbatim
			WithAdditionalArgs([]string{"ARG1=value1"}),
			// Expect WithProperties to quote the values
			WithProperties(map[string]string{"ARG2": "value2"}),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})

	t.Run("uninstall with args", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}
		expectedCmdLine := fmt.Sprintf(`"%s" /x "test.msi" /qn /norestart /log "test.log"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			Uninstall(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})

	t.Run("admin install", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}
		expectedCmdLine := fmt.Sprintf(`"%s" /a "test.msi" /qn /norestart /log "test.log"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			AdministrativeInstall(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})

	t.Run("install args with spaces", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}

		expectedCmdLine := fmt.Sprintf(`"%s" /i "test.msi" /qn /norestart /log "test.log" ARG1="value 1" ARG2="value2" DDAGENTUSER_NAME="NT AUTHORITY\SYSTEM" DDAGENTUSER_PASSWORD="password is long" MSIFASTINSTALL="7"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			Install(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			WithDdAgentUserName(`NT AUTHORITY\SYSTEM`),
			WithDdAgentUserPassword("password is long"),
			WithProperties(map[string]string{"ARG1": "value 1", "ARG2": "value2"}),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		err = cmd.Run(t.Context())
		assert.NoError(t, err)
		mockRunner.AssertExpectations(t)
	})

	t.Run("install args with escaped quotes", func(t *testing.T) {
		mockRunner := &mockCmdRunner{}

		expectedCmdLine := fmt.Sprintf(`"%s" /i "test.msi" /qn /norestart /log "test.log" ARG1="value has ""quotes""" ARG2="value2" DDAGENTUSER_NAME="NT AUTHORITY\SYSTEM" DDAGENTUSER_PASSWORD="password has ""quotes""" MSIFASTINSTALL="7"`, msiexecPath)
		mockRunner.On("Run", msiexecPath, expectedCmdLine).Return(nil)

		cmd, err := Cmd(
			Install(),
			WithMsi("test.msi"),
			WithLogFile("test.log"),
			WithDdAgentUserName(`NT AUTHORITY\SYSTEM`),
			WithDdAgentUserPassword(`password has "quotes"`),
			// Expect quotes to be double escaped
			WithProperties(map[string]string{"ARG1": `value has "quotes"`, "ARG2": "value2"}),
			withCmdRunner(mockRunner),
		)
		require.NoError(t, err)

		err = cmd.Run(t.Context())
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

// TestMsiexecError_ErrorHandling tests that Run returns an MsiexecError that contains the processed log and exit code.
func TestMsiexecError_ErrorHandling(t *testing.T) {
	mockRunner := &mockCmdRunner{}
	mockRunner.On("Run", mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(&mockExitError{code: 1603}).Once()

	// Create a temporary log file with some test content
	testLogContent := "CA: Test error occurred\nDatadog.CustomActions error\nSystem.Exception details"
	logFile := createTestLogFile(t, "test.log", []byte(testLogContent))

	cmd, err := Cmd(
		Install(),
		WithMsi("test.msi"),
		WithLogFile(logFile),
		withCmdRunner(mockRunner),
	)
	require.NoError(t, err)

	err = cmd.Run(t.Context())
	assert.Error(t, err)

	// Check that the error is of type MsiexecError
	var msiErr *MsiexecError
	assert.ErrorAs(t, err, &msiErr)
	// Check that the log file bytes are included
	assert.NotEmpty(t, msiErr.ProcessedLog)
	assert.Contains(t, msiErr.ProcessedLog, "Datadog.CustomActions")

	// Check that the error message is preserved
	assert.Contains(t, msiErr.Error(), "exit status 1603", "error message should be preserved")
	var exitError exitCodeError
	assert.ErrorAs(t, msiErr, &exitError)
	assert.Equal(t, 1603, exitError.ExitCode())

	mockRunner.AssertExpectations(t)
}

// createTestLogFile creates a test log file with the given filename and log data and returns the path.
//
// The file is deleted when the test is done.
//
// The function encodes the log data as UTF-16 with BOM, as expected by openAndProcessLogFile.
func createTestLogFile(t *testing.T, filename string, logData []byte) string {
	logFile := filepath.Join(t.TempDir(), filename)
	logData, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes(logData)
	require.NoError(t, err)
	os.WriteFile(logFile, logData, 0644)
	return logFile
}

// runCmdWithExitCode runs a real process that exits with the provided exit code
// and returns the resulting error (nil for 0, exec.ExitError for non-zero).
func runCmdWithExitCode(t *testing.T, exitCode int) error {
	t.Helper()
	cmdPath := filepath.Join(system32Path, "cmd.exe")
	runner := newRealCmdRunner()
	return runner.Run(cmdPath, fmt.Sprintf("%s /c exit %d", cmdPath, exitCode))
}

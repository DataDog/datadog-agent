// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countOpenFDs returns the number of open file descriptors for the current process
func countOpenFDs(t *testing.T) int {
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	return len(entries)
}

// testCase represents a test case for path validation
type testCase struct {
	name          string
	path          string
	allowedPrefix string
	expectError   bool
	errorContains string
}

// testCaseWithFile represents a test case that requires file system setup
type testCaseWithFile struct {
	name          string
	allowedPrefix string
	setupFunc     func(t *testing.T, testDir string) string
	expectError   bool
	errorContains string
}

func TestIsAllowed(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		allowedPrefix string
		expected      bool
	}{
		// .log extension tests
		{
			name:          "file with .log extension (lowercase)",
			path:          "/etc/application.log",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with .Log extension (mixed case)",
			path:          "/etc/application.Log",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with .log extension in nested path",
			path:          "/opt/app/data/debug.log",
			allowedPrefix: "/var/log/",
			expected:      true,
		},

		// logs directory tests (case insensitive)
		{
			name:          "file with direct parent named logs (lowercase)",
			path:          "/databricks/driver/logs/stdout",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with direct parent named Logs (mixed case)",
			path:          "/var/Logs/app.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with logs in nested path",
			path:          "/opt/app/data/logs/metrics.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with logs in ancestor directory",
			path:          "/logs/app/subdir/debug.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with direct parent logs at root level",
			path:          "/logs/file.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file in subdirectory of logs directory",
			path:          "/databricks/logs/driver/stdout",
			allowedPrefix: "/var/log/",
			expected:      true,
		},

		// allowed prefix tests
		{
			name:          "file directly in allowed prefix",
			path:          "/var/log/syslog",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file in subdirectory of allowed prefix",
			path:          "/var/log/apache2/error.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file in allowed prefix without trailing slash",
			path:          "/tmp/testfile",
			allowedPrefix: "/tmp",
			expected:      true,
		},
		{
			name:          "file in different allowed prefix",
			path:          "/opt/custom/logs/app.txt",
			allowedPrefix: "/opt/custom/",
			expected:      true,
		},

		// negative tests
		{
			name:          "file without .log, not in logs dir, outside prefix",
			path:          "/etc/passwd",
			allowedPrefix: "/var/log/",
			expected:      false,
		},
		{
			name:          "file with logs in filename only",
			path:          "/etc/logserver.conf",
			allowedPrefix: "/var/log/",
			expected:      false,
		},
		{
			name:          "file with logs as prefix in parent directory name",
			path:          "/var/logstash/data.txt",
			allowedPrefix: "/var/log/",
			expected:      false,
		},
		{
			name:          "file with logs as suffix in parent directory name",
			path:          "/var/syslogs/data.txt",
			allowedPrefix: "/var/log/",
			expected:      false,
		},
		{
			name:          "file with log (singular) as directory name",
			path:          "/var/log/data.txt",
			allowedPrefix: "/tmp/",
			expected:      false,
		},
		{
			name:          "file outside all allowed conditions",
			path:          "/home/user/documents/file.txt",
			allowedPrefix: "/var/log/",
			expected:      false,
		},

		// edge cases
		{
			name:          "file with .log extension in logs directory",
			path:          "/opt/logs/app.log",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file with .log extension in allowed prefix",
			path:          "/var/log/system.log",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "file in logs directory within allowed prefix",
			path:          "/var/log/logs/file.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "path with multiple logs directories",
			path:          "/logs/app/logs/debug.txt",
			allowedPrefix: "/var/log/",
			expected:      true,
		},
		{
			name:          "empty allowed prefix should still check other conditions",
			path:          "/opt/logs/app.txt",
			allowedPrefix: "",
			expected:      true,
		},
		{
			name:          "file starting with allowed prefix substring but not matching",
			path:          "/var/log2/file.txt",
			allowedPrefix: "/var/log/",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAllowed(tt.path, tt.allowedPrefix)
			assert.Equal(t, tt.expected, result, "isLogFile(%q, %q) = %v, expected %v", tt.path, tt.allowedPrefix, result, tt.expected)
		})
	}
}

func TestValidateAndOpenWithPrefix(t *testing.T) {
	tests := []testCase{
		{
			name:          "empty path should fail",
			path:          "",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "empty file path provided",
		},
		{
			name:          "relative path should fail",
			path:          "relative/path.log",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "relative path not allowed: relative/path.log",
		},
		{
			name:          "relative path with dot should fail",
			path:          "./relative/path.log",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "relative path not allowed: ./relative/path.log",
		},
		{
			name:          "relative path with parent should fail",
			path:          "../relative/path.log",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "relative path not allowed: ../relative/path.log",
		},
		{
			name:          "non-log file outside allowed prefix should fail",
			path:          "/etc/passwd",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "non-log file not allowed: /etc/passwd",
		},
		{
			name:          "non-log file in allowed prefix should not fail",
			path:          "/var/log/foo",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "log file anywhere should be allowed",
			path:          "/etc/application.log",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "log file with uppercase extension should be allowed",
			path:          "/etc/application.LOG",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "log file with mixed case extension should be allowed",
			path:          "/etc/application.Log",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "non-log file in different allowed prefix should be allowed",
			path:          "/tmp/testfile",
			allowedPrefix: "/tmp/",
			expectError:   false,
		},
		{
			name:          "non-log file outside different allowed prefix should fail",
			path:          "/etc/passwd",
			allowedPrefix: "/tmp/",
			expectError:   true,
			errorContains: "non-log file not allowed: /etc/passwd",
		},
		{
			name:          "file in logs directory should be allowed (databricks example)",
			path:          "/databricks/driver/logs/stdout",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "file in Logs directory should be allowed (mixed case)",
			path:          "/opt/service/Logs/debug.txt",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "file with direct parent logs should be allowed",
			path:          "/opt/app/data/logs/metrics.txt",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "file in subdirectory of logs should be allowed",
			path:          "/app/logs/driver/stdout",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
		{
			name:          "file with logs in filename should not be allowed",
			path:          "/etc/logserver",
			allowedPrefix: "/var/log/",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := validateAndOpenWithPrefix(tt.path, tt.allowedPrefix, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, file)
			} else {
				// For non-error cases, we expect the file to not exist (since we're not creating real files)
				// but the validation should pass
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to resolve path")
				assert.Nil(t, file)
			}
		})
	}
}

func TestValidateAndOpen(t *testing.T) {
	tests := []testCase{
		{
			name:          "empty path should fail",
			path:          "",
			expectError:   true,
			errorContains: "empty file path provided",
		},
		{
			name:          "relative path should fail",
			path:          "relative/path.log",
			expectError:   true,
			errorContains: "relative path not allowed: relative/path.log",
		},
		{
			name:          "non-log file outside /var/log should fail",
			path:          "/etc/passwd",
			expectError:   true,
			errorContains: "non-log file not allowed: /etc/passwd",
		},
		{
			name:        "non-log file in /var/log should not fail",
			path:        "/var/log/bar",
			expectError: false,
		},
		{
			name:        "log file anywhere should be allowed",
			path:        "/etc/application.log",
			expectError: false,
		},
		{
			name:        "file in logs directory should be allowed",
			path:        "/databricks/driver/logs/stdout",
			expectError: false,
		},
		{
			name:        "file in logs directory (uppercase) should be allowed",
			path:        "/app/LOGS/debug.txt",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := validateAndOpen(tt.path)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, file)
			} else {
				// For non-error cases, we expect the file to not exist (since we're not creating real files)
				// but the validation should pass
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to resolve path")
				assert.Nil(t, file)
			}
		})
	}
}

func assertPathInError(t *testing.T, err error, filePath string) {
	resolvedPath, _ := filepath.EvalSymlinks(filePath)
	if resolvedPath != "" {
		assert.Contains(t, err.Error(), resolvedPath)
	} else {
		assert.Contains(t, err.Error(), filePath)
	}
}

func TestValidateAndOpenWithPrefixWithRealFiles(t *testing.T) {
	// Create a temporary directory for testing
	testDir := t.TempDir()

	tests := []testCaseWithFile{
		{
			name:          "regular log file should succeed",
			allowedPrefix: "/var/log/",
			setupFunc: func(t *testing.T, testDir string) string {
				logFile := filepath.Join(testDir, "test.log")
				err := os.WriteFile(logFile, []byte("test content"), 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError: false,
		},
		{
			name:          "regular non-log file in allowed prefix should not fail",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				regularFile := filepath.Join(testDir, "testfile")
				err := os.WriteFile(regularFile, []byte("test content"), 0644)
				require.NoError(t, err)
				return regularFile
			},
			expectError: false,
		},
		{
			name:          "directory should fail",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				dir := filepath.Join(testDir, "testdir")
				err := os.Mkdir(dir, 0755)
				require.NoError(t, err)
				return dir
			},
			expectError:   true,
			errorContains: "not a regular file",
		},
		{
			name:          "symlink to regular file should succeed for log files",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				// Create a regular file
				regularFile := filepath.Join(testDir, "target.log")
				err := os.WriteFile(regularFile, []byte("test content"), 0644)
				require.NoError(t, err)

				// Create a symlink to it
				symlinkFile := filepath.Join(testDir, "link.log")
				err = os.Symlink(regularFile, symlinkFile)
				require.NoError(t, err)

				return symlinkFile
			},
			expectError: false,
		},
		{
			name:          "symlink to regular file from .log should fail",
			allowedPrefix: "/var/log/",
			setupFunc: func(t *testing.T, testDir string) string {
				// Create a regular file
				regularFile := filepath.Join(testDir, "target")
				err := os.WriteFile(regularFile, []byte("test content"), 0644)
				require.NoError(t, err)

				// Create a symlink to it named .log
				symlinkFile := filepath.Join(testDir, "fake.log")
				err = os.Symlink(regularFile, symlinkFile)
				require.NoError(t, err)

				return symlinkFile
			},
			expectError:   true,
			errorContains: "non-log file not allowed",
		},
		{
			name:          "symlink to regular file should not fail in the allowed prefix",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				// Create a regular file
				regularFile := filepath.Join(testDir, "target")
				err := os.WriteFile(regularFile, []byte("test content"), 0644)
				require.NoError(t, err)

				// Create a symlink to it
				symlinkFile := filepath.Join(testDir, "link")
				err = os.Symlink(regularFile, symlinkFile)
				require.NoError(t, err)

				return symlinkFile
			},
			expectError: false,
		},
		{
			name:          "symlink to directory should fail",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				// Create a directory
				dir := filepath.Join(testDir, "targetdir")
				err := os.Mkdir(dir, 0755)
				require.NoError(t, err)

				// Create a symlink to it with unique name
				symlinkFile := filepath.Join(testDir, "link_dir.log")
				err = os.Symlink(dir, symlinkFile)
				require.NoError(t, err)

				return symlinkFile
			},
			expectError:   true,
			errorContains: "not a regular file",
		},
		{
			name:          "broken symlink should fail",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				// Create a broken symlink
				symlinkFile := filepath.Join(testDir, "broken.log")
				err := os.Symlink("/nonexistent/file", symlinkFile)
				require.NoError(t, err)

				return symlinkFile
			},
			expectError:   true,
			errorContains: "failed to resolve path",
		},
		{
			name:          "text file with valid UTF-8 should succeed",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				logFile := filepath.Join(testDir, "text.log")
				content := "Hello, World! This is a valid UTF-8 text file.\n"
				err := os.WriteFile(logFile, []byte(content), 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError: false,
		},
		{
			name:          "binary file should fail isTextFile check",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				logFile := filepath.Join(testDir, "binary.log")
				// Create binary content (non-UTF-8)
				binaryContent := []byte{0xFF, 0xFE, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
				err := os.WriteFile(logFile, binaryContent, 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError:   true,
			errorContains: "not a text file",
		},
		{
			name:          "empty file should be considered text file",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				logFile := filepath.Join(testDir, "empty.log")
				// Create an empty file
				err := os.WriteFile(logFile, []byte{}, 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError: false,
		},
		{
			name:          "file with mixed content should fail isTextFile check",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				logFile := filepath.Join(testDir, "mixed.log")
				// Create content that starts with valid UTF-8 but contains invalid bytes
				content := []byte("Valid UTF-8 start")
				content = append(content, 0xFF, 0xFE, 0x00) // Invalid UTF-8 bytes
				err := os.WriteFile(logFile, content, 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError:   true,
			errorContains: "not a text file",
		},
		{
			name:          "file in Logs directory should succeed (mixed case)",
			allowedPrefix: "/var/log/",
			setupFunc: func(t *testing.T, testDir string) string {
				parentDir := filepath.Join(testDir, "testlogs3")
				err := os.Mkdir(parentDir, 0755)
				require.NoError(t, err)
				logsDir := filepath.Join(parentDir, "Logs")
				err = os.Mkdir(logsDir, 0755)
				require.NoError(t, err)
				logFile := filepath.Join(logsDir, "app.txt")
				err = os.WriteFile(logFile, []byte("app content"), 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError: false,
		},
		{
			name:          "file in nested logs directory should succeed",
			allowedPrefix: "/var/log/",
			setupFunc: func(t *testing.T, testDir string) string {
				nestedDir := filepath.Join(testDir, "app", "data")
				err := os.MkdirAll(nestedDir, 0755)
				require.NoError(t, err)
				logsDir := filepath.Join(nestedDir, "logs")
				err = os.Mkdir(logsDir, 0755)
				require.NoError(t, err)
				logFile := filepath.Join(logsDir, "metrics.txt")
				err = os.WriteFile(logFile, []byte("metrics data"), 0644)
				require.NoError(t, err)
				return logFile
			},
			expectError: false,
		},
		{
			name:          "symlink to file in logs directory should succeed",
			allowedPrefix: "/var/log/",
			setupFunc: func(t *testing.T, testDir string) string {
				parentDir := filepath.Join(testDir, "testlogs6")
				err := os.Mkdir(parentDir, 0755)
				require.NoError(t, err)
				logsDir := filepath.Join(parentDir, "logs")
				err = os.Mkdir(logsDir, 0755)
				require.NoError(t, err)

				// Create target file
				targetFile := filepath.Join(logsDir, "target.txt")
				err = os.WriteFile(targetFile, []byte("target content"), 0644)
				require.NoError(t, err)

				// Create relative symlink - EvalSymlinks resolves it, then
				// openPathSecure opens the resolved path
				symlinkFile := filepath.Join(logsDir, "link.txt")
				err = os.Symlink("target.txt", symlinkFile)
				require.NoError(t, err)

				return symlinkFile
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t, testDir)

			file, err := validateAndOpenWithPrefix(filePath, tt.allowedPrefix, nil)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
					assertPathInError(t, err, filePath)
				}
				assert.Nil(t, file)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, file)
				file.Close()
			}
		})
	}
}

func TestValidateAndOpenWithPrefixPathTraversal(t *testing.T) {
	testDir := t.TempDir()

	tests := []testCaseWithFile{
		{
			name:          "path with .. should be handled safely for log files",
			allowedPrefix: testDir,
			setupFunc: func(t *testing.T, testDir string) string {
				// Create a subdirectory
				subDir := filepath.Join(testDir, "subdir")
				err := os.Mkdir(subDir, 0755)
				require.NoError(t, err)

				// Create a log file in the subdirectory
				logFile := filepath.Join(subDir, "test.log")
				err = os.WriteFile(logFile, []byte("test content"), 0644)
				require.NoError(t, err)

				// Create a symlink that points to the parent directory
				symlinkDir := filepath.Join(subDir, "parent")
				err = os.Symlink(testDir, symlinkDir)
				require.NoError(t, err)

				// Path that would traverse through the symlink
				traversalPath := filepath.Join(symlinkDir, "subdir", "test.log")
				return traversalPath
			},
			expectError: false, // Log files are allowed anywhere
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t, testDir)

			file, err := validateAndOpenWithPrefix(filePath, tt.allowedPrefix, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, file)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, file)
				file.Close()
			}
		})
	}
}

func TestValidateAndOpenWithPrefixTOCTOUPrefixSymlink(t *testing.T) {
	testDir := t.TempDir()

	varDir := filepath.Join(testDir, "var")
	err := os.Mkdir(varDir, 0755)
	require.NoError(t, err)

	logDir := filepath.Join(varDir, "log")
	err = os.Mkdir(logDir, 0755)
	require.NoError(t, err)

	// Create a file
	logFile := filepath.Join(logDir, "shadow")
	err = os.WriteFile(logFile, []byte("syslog content"), 0644)
	require.NoError(t, err)

	// Create /etc simulation
	etcDir := filepath.Join(testDir, "etc")
	err = os.Mkdir(etcDir, 0755)
	require.NoError(t, err)

	sensitiveFile := filepath.Join(etcDir, "shadow")
	err = os.WriteFile(sensitiveFile, []byte("sensitive"), 0644)
	require.NoError(t, err)

	toctouCalled := false
	toctou := func() {
		toctouCalled = true

		// Replace /var/log with symlink to /etc
		require.NoError(t, os.Remove(logFile))
		require.NoError(t, os.Remove(logDir))
		require.NoError(t, os.Symlink(etcDir, logDir))
	}

	file, err := validateAndOpenWithPrefix(logFile, logDir, toctou)

	// openPathWithoutSymlinks should prevent this attack
	assert.Error(t, err)
	assert.ErrorContains(t, err, "failed to open directory component log")
	assert.Nil(t, file)
	assert.True(t, toctouCalled)
}

func TestValidateAndOpenWithPrefixTOCTOUFileSymlink(t *testing.T) {
	testDir := t.TempDir()

	appDir := filepath.Join(testDir, "app")
	err := os.Mkdir(appDir, 0755)
	require.NoError(t, err)

	// Create a legitimate file in logs directory
	logFile := filepath.Join(appDir, "foo.log")
	err = os.WriteFile(logFile, []byte("log content"), 0644)
	require.NoError(t, err)

	sensitiveFile := filepath.Join(appDir, "foo.nonlog")
	err = os.WriteFile(sensitiveFile, []byte("non-log content"), 0644)
	require.NoError(t, err)

	toctouCalled := false
	toctou := func() {
		toctouCalled = true

		// Replace foo.log with a symlink to foo.nonlog
		require.NoError(t, os.Remove(logFile))
		require.NoError(t, os.Symlink(sensitiveFile, logFile))
	}

	file, err := validateAndOpenWithPrefix(logFile, "/var/log/", toctou)

	assert.Error(t, err)
	// This is the error when a symlink is attempted to be opened with O_NOFOLLOW
	assert.ErrorIs(t, err, syscall.ELOOP)
	assert.Nil(t, file)
	assert.True(t, toctouCalled)
}

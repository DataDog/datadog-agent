// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			errorContains: "relative path not allowed",
		},
		{
			name:          "relative path with dot should fail",
			path:          "./relative/path.log",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "relative path not allowed",
		},
		{
			name:          "relative path with parent should fail",
			path:          "../relative/path.log",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "relative path not allowed",
		},
		{
			name:          "non-log file outside allowed prefix should fail",
			path:          "/etc/passwd",
			allowedPrefix: "/var/log/",
			expectError:   true,
			errorContains: "non-log file not allowed",
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
			errorContains: "non-log file not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := validateAndOpenWithPrefix(tt.path, tt.allowedPrefix)

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
			errorContains: "relative path not allowed",
		},
		{
			name:          "non-log file outside /var/log should fail",
			path:          "/etc/passwd",
			expectError:   true,
			errorContains: "non-log file not allowed",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tt.setupFunc(t, testDir)

			file, err := validateAndOpenWithPrefix(filePath, tt.allowedPrefix)

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

			file, err := validateAndOpenWithPrefix(filePath, tt.allowedPrefix)

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

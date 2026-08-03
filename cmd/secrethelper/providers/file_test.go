// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// All these are defined according to the layout materialised by setupReadSecretFileTree.
var fileThatExists = "secret1"
var contentsOfFileThatExists = "secret1-value"
var fileThatDoesNotExist = "secret2"
var fileThatExceedsMaxSize = "secret3"
var fileThatIsASymlinkToSameDir = "secret4" // Points to "secret1"
var fileThatIsASymlinkToOtherDir = "secret5"

// setupReadSecretFileTree materialises the read-secrets layout in a temp dir.
// We build it from code instead of reading checked-in testdata so the test
// behaves identically under `go test` and `bazel test` (Bazel stages testdata
// via symlinks, which trips the symlink-safety check in ReadSecretFile).
func setupReadSecretFileTree(t *testing.T) (testDataAbsPath, testSecretsAbsPath string) {
	t.Helper()

	testDataAbsPath = t.TempDir()
	testSecretsAbsPath = filepath.Join(testDataAbsPath, "read-secrets")
	require.NoError(t, os.Mkdir(testSecretsAbsPath, 0o755))

	require.NoError(t, os.WriteFile(
		filepath.Join(testSecretsAbsPath, fileThatExists),
		[]byte(contentsOfFileThatExists), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(testSecretsAbsPath, fileThatExceedsMaxSize),
		make([]byte, maxSecretFileSize+1), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(testDataAbsPath, "read-secrets-secret5-target"),
		[]byte("symlink-target-value"), 0o600))

	if runtime.GOOS == "windows" {
		return
	}
	require.NoError(t, os.Symlink(fileThatExists,
		filepath.Join(testSecretsAbsPath, fileThatIsASymlinkToSameDir)))
	require.NoError(t, os.Symlink("../read-secrets-secret5-target",
		filepath.Join(testSecretsAbsPath, fileThatIsASymlinkToOtherDir)))
	return
}

func TestReadSecretFile(t *testing.T) {
	testDataAbsPath, testSecretsAbsPath := setupReadSecretFileTree(t)

	tests := []struct {
		name                string
		inputFile           string
		expectedSecretValue string
		expectedError       string
		skipWindows         bool
	}{
		{
			name:                "file exists",
			inputFile:           fileThatExists,
			expectedSecretValue: contentsOfFileThatExists,
		},
		{
			name:          "file does not exist",
			inputFile:     fileThatDoesNotExist,
			expectedError: "secret does not exist",
		},
		{
			name:          "file exceeds max allowed size",
			inputFile:     fileThatExceedsMaxSize,
			expectedError: "secret exceeds max allowed size",
		},
		{
			name:                "file is a symlink pointing to the same dir",
			inputFile:           fileThatIsASymlinkToSameDir,
			expectedSecretValue: contentsOfFileThatExists,
			skipWindows:         true,
		},
		{
			name:      "file is a symlink to other dir",
			inputFile: fileThatIsASymlinkToOtherDir,
			expectedError: fmt.Sprintf(
				"not following symlink \"%s\" outside of \"%s\"",
				testDataAbsPath+"/read-secrets-secret5-target",
				testSecretsAbsPath,
			),
			skipWindows: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.skipWindows && runtime.GOOS == "windows" {
				t.Skip("skipped on windows")
			}

			secret := ReadSecretFile(filepath.Join(testSecretsAbsPath, test.inputFile))

			if test.expectedError != "" {
				assert.Equal(t, test.expectedError, secret.ErrorMsg)
			} else {
				assert.Equal(t, test.expectedSecretValue, secret.Value)
			}
		})
	}
}

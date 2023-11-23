// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build secrets

package providers

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testDataPath = "../testdata"
var testSecretsPath = testDataPath + "/read-secrets"

// All these are defined according to the test files in testSecretsDir
var fileThatExists = "secret1"
var contentsOfFileThatExists = "secret1-value"
var fileThatDoesNotExist = "secret2"
var fileThatExceedsMaxSize = "secret3"
var fileThatIsASymlinkToSameDir = "secret4" // Points to "secret1"
var fileThatIsASymlinkToOtherDir = "secret5"

func TestReadSecretFile(t *testing.T) {
	testDataAbsPath, err := filepath.Abs(testDataPath)
	assert.NoError(t, err)
	testSecretsAbsPath, err := filepath.Abs(testSecretsPath)
	assert.NoError(t, err)

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

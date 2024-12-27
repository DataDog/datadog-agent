// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"os"
	"path/filepath"
	"strings"

	"testing"
)

type testLogger struct {
	t *testing.T
}

func newTestLogger(t *testing.T) testLogger {
	return testLogger{t: t}
}

func (tl testLogger) Write(p []byte) (n int, err error) {
	tl.t.Helper()
	tl.t.Log(string(p))
	return len(p), nil
}

// CreateTestOutputDir creates a directory for a specific test that can be used to store output files and artifacts.
// The test name is used in the directory name, and invalid characters are replaced with underscores.
//
// Example:
//   - test name: TestInstallSuite/TestInstall/install_version=7.50.0
//   - output directory: <root>/TestInstallSuite/TestInstall/install_version_7_50_0
func CreateTestOutputDir(root string, t *testing.T) (string, error) {
	// https://en.wikipedia.org/wiki/Filename#Reserved_characters_and_words
	invalidPathChars := strings.Join([]string{"?", "%", "*", ":", "|", "\"", "<", ">", ".", ",", ";", "="}, "")

	testPart := strings.ReplaceAll(t.Name(), invalidPathChars, "_")
	path := filepath.Join(root, testPart)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return "", err
	}
	return path, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"strings"

	"testing"
)

// TestLogger is a logger that writes to the test log.
type TestLogger struct {
	t *testing.T
}

// NewTestLogger creates a new TestLogger that writes to the test log.
func NewTestLogger(t *testing.T) TestLogger {
	return TestLogger{t: t}
}

// Write writes the given bytes to the test log.
func (tl TestLogger) Write(p []byte) (n int, err error) {
	tl.t.Helper()
	tl.t.Log(string(p))
	return len(p), nil
}

// SanitizeDirectoryName replace invalid characters in a directory name underscores.
//
// Example:
//   - name: TestInstallSuite/TestInstall/install_version=7.50.0
//   - output directory: <root>/TestInstallSuite/TestInstall/install_version_7_50_0
func SanitizeDirectoryName(name string) string {
	// https://en.wikipedia.org/wiki/Filename#Reserved_characters_and_words
	invalidPathChars := strings.Join([]string{"?", "%", "*", ":", "|", "\"", "<", ">", ".", ",", ";", "="}, "")
	return strings.ReplaceAll(name, invalidPathChars, "_")
}

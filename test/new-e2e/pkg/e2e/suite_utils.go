// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"

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

// CreateRootOutputDir creates and returns a directory for tests to store output files and artifacts.
// A timestamp is included in the path to distinguish between multiple runs, and os.MkdirTemp() is
// used to avoid name collisions between parallel runs.
//
// A new directory is created on each call to this function, it is recommended to save this result
// and use it for all tests in a run. For example see BaseSuite.GetRootOutputDir().
//
// See runner.GetProfile().GetOutputDir() for the root output directory selection logic.
//
// See CreateTestOutputDir and BaseSuite.CreateTestOutputDir for a function that returns a subdirectory for a specific test.
func CreateRootOutputDir() (string, error) {
	outputRoot, err := runner.GetProfile().GetOutputDir()
	if err != nil {
		return "", err
	}
	// Append timestamp to distinguish between multiple runs
	// Format: YYYY-MM-DD_HH-MM-SS
	// Use a custom timestamp format because Windows paths can't contain ':' characters
	// and we don't need the timezone information.
	timePart := time.Now().Format("2006-01-02_15-04-05")
	// create root directory
	err = os.MkdirAll(outputRoot, 0755)
	if err != nil {
		return "", err
	}
	// Create final output directory
	// Use MkdirTemp to avoid name collisions between parallel runs
	outputRoot, err = os.MkdirTemp(outputRoot, fmt.Sprintf("%s_*", timePart))
	if err != nil {
		return "", err
	}
	if os.Getenv("CI") == "" {
		// Create a symlink to the latest run for user convenience
		// TODO: Is there a standard "ci" vs "local" check?
		//       This code used to be in localProfile.GetOutputDir()
		latestLink := filepath.Join(filepath.Dir(outputRoot), "latest")
		// Remove the symlink if it already exists
		if _, err := os.Lstat(latestLink); err == nil {
			err = os.Remove(latestLink)
			if err != nil {
				return "", err
			}
		}
		err = os.Symlink(outputRoot, latestLink)
		if err != nil {
			return "", err
		}
	}
	return outputRoot, nil
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

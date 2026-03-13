// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunMissingPath(t *testing.T) {
	exitCode := run([]string{
		"worksynchronizer",
		"--modules-file", "testdata/modules.yml",
	})
	assert.NotEqual(t, 0, exitCode, "should fail when --path is omitted")
}

func TestRunMissingModulesFile(t *testing.T) {
	exitCode := run([]string{
		"worksynchronizer",
		"--path", "testdata/in.go.work",
	})
	assert.NotEqual(t, 0, exitCode, "should fail when --modules-file is omitted")
}

func TestRunSameGoWork(t *testing.T) {
	tmpDir := t.TempDir()
	workPath := filepath.Join(tmpDir, "go.work")

	inputContent, err := os.ReadFile("testdata/in.go.work")
	require.NoError(t, err)
	err = os.WriteFile(workPath, inputContent, 0644)
	require.NoError(t, err)

	exitCode := run([]string{
		"worksynchronizer",
		"--path", workPath,
		"--modules-file", "testdata/modules.yml",
	})
	require.Equal(t, 0, exitCode)

	assertOutputMatchesExpected(t, workPath)
}

func TestRunSeparateGoWork(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.work")

	exitCode := run([]string{
		"worksynchronizer",
		"--path", "testdata/in.go.work",
		"--modules-file", "testdata/modules.yml",
		"--output", outputPath,
	})
	require.Equal(t, 0, exitCode)

	_, err := os.Stat(outputPath)
	require.NoError(t, err, "output file should exist at specified path")

	assertOutputMatchesExpected(t, outputPath)
}

func assertOutputMatchesExpected(t *testing.T, outputPath string) {
	t.Helper()

	actualContent, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	expectedContent, err := os.ReadFile("testdata/expected.go.work")
	require.NoError(t, err)

	assert.Equal(t, string(expectedContent), string(actualContent), "output should match expected format (no spurious newlines)")
}

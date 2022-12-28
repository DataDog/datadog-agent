// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package helpers

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createPermsTestFile(t *testing.T) (string, string, string, string) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "file1")
	f2 := filepath.Join(tempDir, "file2")
	f3 := filepath.Join(tempDir, "file3")

	os.WriteFile(f1, nil, 0666)
	os.WriteFile(f2, nil, 0444)

	return tempDir, f1, f2, f3
}

func TestPermsFileCommit(t *testing.T) {
	tempDir, f1, f2, f3 := createPermsTestFile(t)

	// Set current directory to temp (and restore) because
	// filepath.Dir(config.Datadog.ConfigFileUsed()) returns "."
	curDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(curDir)

	pi := permissionsInfos{}
	pi.add(f1)
	pi.add(f2)
	pi.add(f3)

	// No error
	b, err := pi.commit()
	require.NoError(t, err)

	res, _ := regexp.Match("\nC:\\\\.+\\\\file1 ", b)
	assert.True(t, res)
	res, _ = regexp.Match("\nC:\\\\.+\\\\file2 ", b)
	assert.True(t, res)
	res, _ = regexp.Match("\nSuccessfully processed 3 files", b)
	assert.True(t, res)
}

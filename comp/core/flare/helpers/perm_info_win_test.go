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

func createPermsTestFile(t *testing.T) (string, string, string) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "file 1")
	f2 := filepath.Join(tempDir, "file2")
	f3 := filepath.Join(tempDir, "file3")

	os.WriteFile(f1, nil, 0666)
	os.WriteFile(f2, nil, 0444)
	return f1, f2, f3
}

// Test that addParentPerms does not loop on Windows.
func TestPermissionsInfosAddWindows(t *testing.T) {
	permsInfos := make(permissionsInfos)

	// Basic Case
	path := "C:\\a\\b\\c\\d"
	permsInfos.add(path)
	assert.Contains(t, permsInfos, path)
	assert.True(t, len(permsInfos) == 1)
}

func TestPermsFileAdd(t *testing.T) {
	f1, f2, f3 := createPermsTestFile(t)

	pi := permissionsInfos{}

	pi.add(f1)
	pi.add(f2)
	pi.add(f3)

	require.Len(t, pi, 3)

	assert.Equal(t, f1, pi[f1].path)
	assert.Equal(t, "-rw-rw-rw-", pi[f1].mode)

	assert.Equal(t, f2, pi[f2].path)
	assert.Equal(t, "-r--r--r--", pi[f2].mode)

	assert.Equal(t, f3, pi[f3].path)
	assert.Empty(t, pi[f3].mode)
	assert.NotNil(t, pi[f3].err)
}

func TestPermsFileCommit(t *testing.T) {
	f1, f2, f3 := createPermsTestFile(t)

	pi := permissionsInfos{}
	pi.add(f1)
	pi.add(f2)
	pi.add(f3)

	// No error
	b, err := pi.commit()
	require.NoError(t, err)

	// All files are reportes
	res, _ := regexp.Match("\nFile:.+file 1\n", b)
	assert.True(t, res)
	res, _ = regexp.Match("\nFile:.+file2\n", b)
	assert.True(t, res)
	res, _ = regexp.Match("\nFile:.+file3\n", b)
	assert.True(t, res)

	// Success and failiures are recorded
	res, _ = regexp.Match("\nSuccessfully processed 1 files", b)
	assert.True(t, res)
	res, _ = regexp.Match("\nError:could not find file.+file3", b)
	assert.True(t, res)
}

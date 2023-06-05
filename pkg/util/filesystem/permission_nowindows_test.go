// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filesystem

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestRemoveAccessToOtherUsers(t *testing.T) {
	p, err := NewPermission()
	require.NoError(t, err)

	root := t.TempDir()

	testFile := filepath.Join(root, "file")
	testDir := filepath.Join(root, "dir")

	err = os.WriteFile(testFile, []byte("test"), 0777)
	require.NoError(t, err)
	err = os.Mkdir(testDir, 0777)
	require.NoError(t, err)

	err = p.RemoveAccessToOtherUsers(testFile)
	require.NoError(t, err)
	stat, err := os.Stat(testFile)
	require.NoError(t, err)
	assert.Equal(t, int(stat.Mode().Perm()), 0700)

	err = p.RemoveAccessToOtherUsers(testDir)
	require.NoError(t, err)
	stat, err = os.Stat(testDir)
	require.NoError(t, err)
	assert.Equal(t, int(stat.Mode().Perm()), 0700)
}

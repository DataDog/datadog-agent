// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build windows
// +build windows

package filesystem

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsCollect(t *testing.T) {
	mountsObj, err := new(FileSystem).Collect()
	require.NoError(t, err)

	mounts, ok := mountsObj.([]interface{})
	require.True(t, ok, "Could not cast %+v to []interface{}", mountsObj)

	require.Greater(t, len(mounts), 0)

	for _, mountObj := range mounts {
		mount := mountObj.(map[string]string)
		assert.NotEmpty(t, mount["name"])

		assert.NotEmpty(t, mount["kb_size"])
		sizeKB, err := strconv.Atoi(mount["kb_size"])
		require.NoError(t, err)
		assert.GreaterOrEqual(t, sizeKB, 0)

		// XXX: Some drives on Windows do not seem to return a value here
		// assert.NotEmpty(t, mount["mounted_on"])
	}
}

func TestWindowsGet(t *testing.T) {
	mounts, err := new(FileSystem).Get()
	require.NoError(t, err)

	require.Greater(t, len(mounts), 0)

	for _, mount := range mounts {
		assert.NotEmpty(t, mount.Name)
		assert.GreaterOrEqual(t, mount.SizeKB, uint64(0))

		// XXX: Some drives on Windows do not seem to return a value here
		// assert.NotEmpty(t, mount.MountedOn)
	}
}

// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package filesystem

import (
	"bytes"
	"encoding/json"
	"errors"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTimeout(t *testing.T) {
	_, err := getWithTimeout(time.Nanosecond, func() ([]MountInfo, error) {
		time.Sleep(5 * time.Second)
		return nil, errors.New("fail")

	})
	require.ErrorIs(t, err, ErrTimeoutExceeded)
}

func TestAsJSON(t *testing.T) {
	mounts, err := CollectInfo()
	require.NoError(t, err)

	marshallable, _, err := mounts.AsJSON()
	require.NoError(t, err)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(marshalled))
	// do not ignore unknown fields
	decoder.DisallowUnknownFields()

	// Any change to this datastructure should be notified to the backend
	// team to ensure compatibility.
	type Filesystem struct {
		KbSize string `json:"kb_size"`
		// MountedOn can be empty on Windows
		MountedOn string `json:"mounted_on"`
		Name      string `json:"name"`
	}

	var filesystems []Filesystem
	err = decoder.Decode(&filesystems)
	require.NoError(t, err)

	// check that we read the full json
	require.False(t, decoder.More())

	require.NotEmpty(t, filesystems)

	for _, filesystem := range filesystems {
		if runtime.GOOS != "windows" {
			// On Windows, MountedOn can be empty
			assert.NotEmpty(t, filesystem.MountedOn)
		}

		sizeKB, err := strconv.Atoi(filesystem.KbSize)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, sizeKB, 0)

		assert.NotEmpty(t, filesystem.Name)
	}
}

func TestCollectInfo(t *testing.T) {
	mounts, err := CollectInfo()
	require.NoError(t, err)

	require.NotEmpty(t, mounts)

	for _, mount := range mounts {
		assert.NotEmpty(t, mount.Name)
		assert.GreaterOrEqual(t, mount.SizeKB, uint64(0))
		if runtime.GOOS != "windows" {
			// On Windows, MountedOn can be empty
			assert.NotEmpty(t, mount.MountedOn)
		}
	}
}

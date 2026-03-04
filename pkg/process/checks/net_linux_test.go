// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRemoteProcessTags_Linux(t *testing.T) {
	t.Run("reads DD_SERVICE from /proc/self/environ", func(t *testing.T) {
		// DD_SERVICE must be set before the process starts for /proc/<pid>/environ
		// to contain it. Since we can't guarantee that, read /proc/self/environ
		// to check if DD_SERVICE is present and adjust expectations accordingly.
		pid := int32(os.Getpid())
		tags := getRemoteProcessTags(pid, nil, nil)

		envData, err := os.ReadFile("/proc/self/environ")
		require.NoError(t, err)

		hasDDService := false
		for _, env := range splitNullTerminated(envData) {
			if len(env) > 11 && env[:11] == "DD_SERVICE=" {
				hasDDService = true
				break
			}
		}

		if hasDDService {
			require.NotNil(t, tags)
			assert.NotEmpty(t, tags)
		} else {
			// No DD_SERVICE in environ — getRemoteProcessTags returns nil
			assert.Nil(t, tags)
		}
	})

	t.Run("returns nil for non-existent PID", func(t *testing.T) {
		// PID that almost certainly doesn't exist
		tags := getRemoteProcessTags(2147483647, nil, nil)
		assert.Nil(t, tags)
	})
}

// splitNullTerminated splits a null-terminated byte slice into strings.
func splitNullTerminated(data []byte) []string {
	var result []string
	start := 0
	for i, b := range data {
		if b == 0 {
			if i > start {
				result = append(result, string(data[start:i]))
			}
			start = i + 1
		}
	}
	return result
}

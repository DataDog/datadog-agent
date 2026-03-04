// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRemoteProcessTags_Windows(t *testing.T) {
	t.Run("returns tags from process cache", func(t *testing.T) {
		procCacheTags := map[uint32][]string{
			100: {"env:prod", "service:web"},
		}
		tags := getRemoteProcessTags(100, procCacheTags, nil)
		assert.Equal(t, []string{"env:prod", "service:web"}, tags)
	})

	t.Run("returns nil for unknown PID", func(t *testing.T) {
		procCacheTags := map[uint32][]string{
			100: {"env:prod"},
		}
		tags := getRemoteProcessTags(999, procCacheTags, nil)
		assert.Nil(t, tags)
	})

	t.Run("returns nil when cache is nil", func(t *testing.T) {
		tags := getRemoteProcessTags(100, nil, nil)
		assert.Nil(t, tags)
	})
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestErrptSince(t *testing.T) {
	assert.Equal(t, "1030134526", errptSince(time.Date(2026, 10, 30, 13, 45, 21, 0, time.UTC)))
	// Year rollover: January dates must carry the previous year's 2-digit value.
	assert.Equal(t, "0102000027", errptSince(time.Date(2027, 1, 2, 0, 0, 0, 0, time.UTC)))
}

func TestBuildErrptLog(t *testing.T) {
	t.Run("with data", func(t *testing.T) {
		content := string(buildErrptLog([]byte("some entry"), nil, []byte("another entry"), nil))
		assert.Contains(t, content, "=== Process deaths by signal")
		assert.Contains(t, content, "=== SRC subsystem lifecycle")
		assert.NotContains(t, content, "flare --local")
		assert.NotContains(t, content, "errpt failed")
	})

	t.Run("empty and successful yields the root hint", func(t *testing.T) {
		content := string(buildErrptLog(nil, nil, nil, nil))
		assert.Contains(t, content, "only readable by root")
		assert.Contains(t, content, "sudo datadog-agent flare --local")
	})

	t.Run("failure is reported inline and suppresses the hint", func(t *testing.T) {
		content := string(buildErrptLog(nil, errors.New("permission denied"), nil, nil))
		assert.Contains(t, content, "(errpt failed: permission denied)")
		assert.NotContains(t, content, "sudo datadog-agent flare --local")
	})

	t.Run("oversized output is truncated", func(t *testing.T) {
		deaths := make([]byte, errptMaxOutput+1024)
		content := buildErrptLog(deaths, nil, nil, nil)
		assert.Contains(t, string(content), "[output truncated]")
		assert.Less(t, len(content), errptMaxOutput+1024)
	})
}

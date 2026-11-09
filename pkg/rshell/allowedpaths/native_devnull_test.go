// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNativeDevNull(t *testing.T) {
	windows := runtime.GOOS == "windows"
	for path, want := range map[string]bool{
		"/dev/null":  !windows,
		"NUL":        windows,
		"nul":        windows,
		"/dev/null/": false,
		"dev/null":   false,
		"./NUL":      false,
		"":           false,
	} {
		assert.Equal(t, want, IsNativeDevNull(path), path)
	}
	// IsDevNull keeps accepting "/dev/null" everywhere for output redirects.
	assert.True(t, IsDevNull("/dev/null"))
}

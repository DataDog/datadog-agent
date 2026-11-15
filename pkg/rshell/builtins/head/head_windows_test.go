// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package head_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHeadWindowsReservedName(t *testing.T) {
	// Windows reserved device names (CON, PRN, AUX, NUL, COM1, LPT1, etc.)
	// must never be opened as files — attempting to do so can hang or behave
	// unexpectedly. The sandbox (AllowedPaths) should block access, resulting
	// in a permission-denied error rather than a hang.
	dir := t.TempDir()
	for _, name := range []string{"CON", "PRN", "AUX", "NUL", "COM1", "LPT1"} {
		t.Run(name, func(t *testing.T) {
			_, stderr, code := cmdRun(t, "head "+name, dir)
			assert.Equal(t, 1, code, "expected failure for reserved name %s", name)
			assert.Contains(t, stderr, "head:", "expected head: prefix in stderr for %s", name)
		})
	}
}

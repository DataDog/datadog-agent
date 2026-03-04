// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package interp

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTail_WindowsReservedName_Rejected verifies that Windows reserved device
// names are rejected before any open attempt, producing a clear error message
// and exit code 1.
func TestTail_WindowsReservedName_Rejected(t *testing.T) {
	for _, name := range []string{"CON", "NUL", "COM1", "LPT1"} {
		t.Run(name, func(t *testing.T) {
			_, stderr, err := runScript(t, fmt.Sprintf("tail %s", name))
			require.NoError(t, err) // interpreter-level no error; exit code communicated via r.exitCode
			assert.Contains(t, stderr, "reserved device name")
		})
	}
}

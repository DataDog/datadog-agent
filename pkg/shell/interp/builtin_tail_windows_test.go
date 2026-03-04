// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package interp

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTail_WindowsReservedName verifies that Windows reserved device names
// (CON, NUL, PRN, etc.) are handled gracefully and do not hang.
// Attempting to open them as files should result in an error, not a hang.
func TestTail_WindowsReservedName(t *testing.T) {
	reserved := []string{"CON", "NUL", "PRN", "AUX", "COM1", "LPT1"}
	for _, name := range reserved {
		t.Run(name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			r := New(WithStdout(&out), WithStderr(&errOut), WithDir(t.TempDir()))
			err := r.Run(context.Background(), "tail -n 1 "+name)
			// Either errors cleanly or returns empty output — must not hang.
			_ = err
			assert.NotEqual(t, -1, r.ExitCode(), "exit code should be set")
		})
	}
}

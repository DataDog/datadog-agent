// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

//go:build windows

package interp_test

import (
	"testing"
)

// TestTailWindowsReservedNames verifies Windows reserved names (CON, NUL, etc.) are
// handled gracefully. The sandbox's OpenFile should reject or fail these safely.
func TestTailWindowsReservedNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"CON", "NUL", "PRN", "AUX", "COM1", "LPT1"} {
		t.Run(name, func(t *testing.T) {
			_, _, exitCode := tailRun(t, "tail "+name, dir)
			// We only assert that it doesn't hang; exit code may vary.
			_ = exitCode
		})
	}
}

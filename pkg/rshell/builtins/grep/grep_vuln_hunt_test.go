// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package grep_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// vuln-hunt 2026-05-19-codex: overflowing numeric flags must fail during
// parsing instead of wrapping into small or negative limits.
func TestVulnHuntBuiltinIntegerOverflow_NumericFlagOverflowRejected(t *testing.T) {
	dir := t.TempDir()
	pentestWriteFile(t, dir, "file.txt", "foo\n")
	huge := strings.Repeat("9", 128)

	for _, flag := range []string{"-m", "-A", "-B", "-C"} {
		t.Run(flag, func(t *testing.T) {
			_, stderr, code := grepRun(t, "grep "+flag+" "+huge+" foo file.txt", dir)
			assert.Equal(t, 1, code)
			assert.Contains(t, stderr, "grep:")
		})
	}
}

// vuln-hunt 2026-05-19-codex: if /dev/zero is explicitly allowed, grep still
// must not block forever on an infinite source without newline delimiters.
func TestVulnHuntBuiltinSpecialFiles_DevZeroBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/dev/zero is Unix-specific")
	}

	dir := t.TempDir()
	mustNotHang(t, func() {
		_, stderr, code := grepRun(t, "grep x /dev/zero", dir, "/dev")
		assert.Equal(t, 2, code)
		assert.Contains(t, stderr, "grep:")
	})
}

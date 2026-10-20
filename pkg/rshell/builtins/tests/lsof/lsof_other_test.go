// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package lsof_test

import (
	"strings"
	"testing"
)

// TestLsofNotSupportedOffLinux verifies that lsof exits 1 with a
// "not supported" message on non-Linux platforms, matching the free
// builtin's documented Linux-only pattern.
func TestLsofNotSupportedOffLinux(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "lsof", nil)
	if code != 1 {
		t.Errorf("exit code = %d, want 1; stdout: %s", code, stdout)
	}
	if !strings.Contains(stderr, "not supported") {
		t.Errorf("stderr = %q, want it to contain %q", stderr, "not supported")
	}
}

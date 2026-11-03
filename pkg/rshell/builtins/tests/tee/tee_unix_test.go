// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package tee_test

import (
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTeeRejectsFIFOWriteTargetNoReader lives in a unix-only file because
// syscall.Mkfifo does not exist on Windows (a build-time symbol, not a
// runtime skip) — see the FIFO write-target rejection notes in
// builtins/tee/tee.go and AGENTS.md.
func TestTeeRejectsFIFOWriteTargetNoReader(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "pipe")
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Skipf("mkfifo not supported: %v", err)
	}
	stdout, stderr, code := teeRunStdin(t, "tee pipe", dir, "data\n")
	assert.Equal(t, 1, code, "tee on a FIFO with no reader must fail, not hang")
	assert.Contains(t, stderr, "not a regular file")
	// stdout must still receive the data even though the FIFO destination
	// was rejected.
	assert.Equal(t, "data\n", stdout)
}

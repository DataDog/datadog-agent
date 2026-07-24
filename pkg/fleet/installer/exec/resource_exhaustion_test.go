// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package exec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsResourceExhaustionCrash(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		// --- fatal-error-anchored runtime.throw() signatures ---
		{
			name:   "pageAlloc out of memory",
			output: "fatal error: pageAlloc: out of memory\n",
			want:   true,
		},
		{
			name:   "heap arena metadata",
			output: "fatal error: out of memory allocating heap arena metadata\n",
			want:   true,
		},
		{
			name:   "heap arena map",
			output: "fatal error: out of memory allocating heap arena map\n",
			want:   true,
		},
		{
			name:   "allArenas",
			output: "fatal error: out of memory allocating allArenas\n",
			want:   true,
		},
		{
			name:   "generic runtime out of memory (linux/bsd/darwin mmap ENOMEM)",
			output: "fatal error: runtime: out of memory\n",
			want:   true,
		},
		{
			name:   "generic windows out of memory (VirtualAlloc ENOMEM/commit-limit)",
			output: "fatal error: out of memory\n",
			want:   true,
		},
		{
			name:   "thread exhaustion",
			output: "runtime: program exceeds 10000-thread limit\nfatal error: thread exhaustion\n",
			want:   true,
		},
		// --- diagnostic / OS-errno-text signatures (no fatal error prefix) ---
		{
			name:   "cannot allocate memory",
			output: "runtime: cannot allocate memory\n",
			want:   true,
		},
		{
			name:   "failed to create new OS thread",
			output: "runtime: failed to create new OS thread (have 10000 already; errno=11)\n",
			want:   true,
		},
		{
			name:   "thread limit print line alone",
			output: "runtime: program exceeds 10000-thread limit\n",
			want:   true,
		},
		{
			name:   "VirtualAlloc failure on windows, errno=1455 (ERROR_COMMITMENT_LIMIT)",
			output: "runtime: VirtualAlloc of 8192 bytes failed with errno=1455\n",
			want:   true,
		},
		{
			name:   "VirtualAlloc failure on windows, errno=8 (ERROR_NOT_ENOUGH_MEMORY)",
			output: "runtime: VirtualAlloc of 8192 bytes failed with errno=8\n",
			want:   true,
		},
		{
			name:   "too much locked memory (mmap EAGAIN, ulimit -l)",
			output: "runtime: mmap: too much locked memory (check 'ulimit -l').\n",
			want:   true,
		},
		{
			name:   "paging file too small",
			output: "The paging file is too small for this operation to complete.\n",
			want:   true,
		},
		// --- negative cases: unrelated errors ---
		{
			name:   "unrelated error",
			output: "exit status 1: package not found",
			want:   false,
		},
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "nil output",
			output: "",
			want:   false,
		},
		// --- negative cases: near-misses that must NOT match ---
		{
			name:   "out of memory mentioned without fatal error prefix",
			output: "warning: process is close to running out of memory, consider increasing limits\n",
			want:   false,
		},
		{
			name:   "benign mention of memory in an unrelated message",
			output: "error: failed to read config: field \"memory_limit_mb\" is not a valid integer\n",
			want:   false,
		},
		{
			name:   "benign mention of thread in an unrelated message",
			output: "panic: goroutine running on wrong thread for this operation: main.worker\n",
			want:   false,
		},
		{
			name:   "fatal error with an unrelated message",
			output: "fatal error: concurrent map writes\n",
			want:   false,
		},
		{
			name:   "path that happens to contain the word memory",
			output: "error: could not open /var/lib/datadog-installer/memory-profile.json: permission denied\n",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output []byte
			if tt.output != "" {
				output = []byte(tt.output)
			}
			assert.Equal(t, tt.want, isResourceExhaustionCrash(output))
		})
	}
}

// TestIsResourceExhaustionCrash_NilInput explicitly exercises a nil (as opposed to empty
// non-nil) byte slice, since callers may pass a bytes.Buffer's Bytes() result before anything
// was ever written to it.
func TestIsResourceExhaustionCrash_NilInput(t *testing.T) {
	assert.False(t, isResourceExhaustionCrash(nil))
}

func TestErrResourceExhaustedIsWrappedWithMultiErrorf(t *testing.T) {
	// Mirrors the fmt.Errorf("run failed: %w: %w", ErrResourceExhausted, err) wrapping used in
	// installerCmd.Run, to confirm errors.Is still finds the sentinel through the wrap.
	underlying := errors.New("exit status 2")
	err := fmt.Errorf("run failed: %w: %w", ErrResourceExhausted, underlying)

	assert.True(t, errors.Is(err, ErrResourceExhausted))
	assert.True(t, errors.Is(err, underlying))
}

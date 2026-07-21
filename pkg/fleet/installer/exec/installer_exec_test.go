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
			name:   "VirtualAlloc failure on windows",
			output: "runtime: VirtualAlloc of 8192 bytes failed with errno=1455\n",
			want:   true,
		},
		{
			name:   "paging file too small",
			output: "The paging file is too small for this operation to complete.\n",
			want:   true,
		},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResourceExhaustionCrash([]byte(tt.output)))
		})
	}
}

func TestErrResourceExhaustedIsWrappedWithMultiErrorf(t *testing.T) {
	// Mirrors the fmt.Errorf("run failed: %w: %w", ErrResourceExhausted, err) wrapping used in
	// installerCmd.Run, to confirm errors.Is still finds the sentinel through the wrap.
	underlying := errors.New("exit status 2")
	err := fmt.Errorf("run failed: %w: %w", ErrResourceExhausted, underlying)

	assert.True(t, errors.Is(err, ErrResourceExhausted))
	assert.True(t, errors.Is(err, underlying))
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package builtins

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
)

func TestIsBrokenPipe(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unix EPIPE", syscall.EPIPE, true},
		{"unrelated error", errors.New("boom"), false},
		{"unrelated errno", syscall.ENOENT, false},
		// os.Pipe writes typically wrap the errno in *os.PathError (Unix)
		// or *os.SyscallError; errors.Is must see through both.
		{"wrapped in PathError", &os.PathError{Op: "write", Path: "|1", Err: syscall.EPIPE}, true},
		{"wrapped generic error", fmt.Errorf("write: %w", syscall.EPIPE), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBrokenPipe(tt.err); got != tt.want {
				t.Errorf("IsBrokenPipe(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

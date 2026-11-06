// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package builtins

import (
	"os"
	"syscall"
	"testing"
)

func TestIsBrokenPipeWindowsErrnos(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"ERROR_BROKEN_PIPE", syscall.Errno(109), true},
		{"ERROR_NO_DATA", syscall.Errno(232), true},
		{"ERROR_BROKEN_PIPE wrapped in PathError", &os.PathError{Op: "write", Path: "|1", Err: syscall.Errno(109)}, true},
		{"ERROR_NO_DATA wrapped in PathError", &os.PathError{Op: "write", Path: "|1", Err: syscall.Errno(232)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBrokenPipe(tt.err); got != tt.want {
				t.Errorf("IsBrokenPipe(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

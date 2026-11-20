// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package testcmd_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func TestTestWindowsReservedNames(t *testing.T) {
	dir := t.TempDir()
	// NUL is the Windows null device (equivalent to /dev/null) and should
	// be reported as existing, just like /dev/null on Unix.
	reserved := []struct {
		name string
		code int
	}{
		{"CON", 1},
		{"PRN", 1},
		{"AUX", 1},
		{"NUL", 0},
		{"COM1", 1},
		{"LPT1", 1},
	}
	for _, tc := range reserved {
		t.Run(tc.name, func(t *testing.T) {
			_, _, code := runScript(t, `test -e `+tc.name, dir, interp.AllowedPaths([]string{dir}))
			assert.Equal(t, tc.code, code)
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package dcv

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDCVExecutableFromServiceCommand(t *testing.T) {
	tests := map[string]struct {
		commandLine string
		expected    string
	}{
		"default path": {
			commandLine: `"C:\Program Files\NICE\DCV\Server\bin\dcvserver.exe"`,
			expected:    `C:\Program Files\NICE\DCV\Server\bin\dcv.exe`,
		},
		"custom path with service arguments": {
			commandLine: `D:\DCV\bin\dcvserver.exe --service`,
			expected:    `D:\DCV\bin\dcv.exe`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			executable, err := dcvExecutableFromServiceCommand(test.commandLine)
			require.NoError(t, err)
			require.Equal(t, test.expected, executable)
		})
	}
}

func TestDCVExecutableFromServiceCommandRejectsUnexpectedPaths(t *testing.T) {
	for name, commandLine := range map[string]string{
		"empty":                   "",
		"relative":                `dcvserver.exe`,
		"different executable":    `C:\DCV\bin\other.exe`,
		"ambiguous unquoted path": `C:\Program Files\NICE\DCV\Server\bin\dcvserver.exe`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := dcvExecutableFromServiceCommand(commandLine)
			require.Error(t, err)
		})
	}
}

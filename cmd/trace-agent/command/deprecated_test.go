// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixDeprecatedFlags(t *testing.T) {
	tests := []struct {
		cli        string
		expectArgs []string
		expectWarn string
	}{
		{
			cli: "-config datadog.yaml",
			expectArgs: []string{
				"--config",
				"datadog.yaml",
			},
		},
		{
			cli: "-config=datadog.yaml",
			expectArgs: []string{
				"--config=datadog.yaml",
			},
		},
		{
			cli: "--config=datadog.yaml",
			expectArgs: []string{
				"--config=datadog.yaml",
			},
		},
		{
			cli: "-pid pidfile",
			expectArgs: []string{
				"--pidfile",
				"pidfile",
			},
		},
		{
			cli: "--pid pidfile",
			expectArgs: []string{
				"--pidfile",
				"pidfile",
			},
		},
		{
			cli: "-cpuprofile file",
			expectArgs: []string{
				"--cpu-profile",
				"file",
			},
		},
		{
			cli: "--cpuprofile file",
			expectArgs: []string{
				"--cpu-profile",
				"file",
			},
		},
		{
			cli: "-memprofile file",
			expectArgs: []string{
				"--mem-profile",
				"file",
			},
		},
		{
			cli: "--memprofile file",
			expectArgs: []string{
				"--mem-profile",
				"file",
			},
		},
		{
			cli: "-info",
			expectArgs: []string{
				"info",
			},
		},
		{
			cli: "--info",
			expectArgs: []string{
				"info",
			},
		},
		{
			cli: "-version",
			expectArgs: []string{
				"version",
			},
		},
		{
			cli: "--version",
			expectArgs: []string{
				"version",
			},
		},
		{
			cli: "-start-service",
			expectArgs: []string{
				"--start-service",
			},
		},
		{
			cli: "-stop-service",
			expectArgs: []string{
				"--stop-service",
			},
		},
		{
			cli: "-foreground",
			expectArgs: []string{
				"--foreground",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.cli, func(t *testing.T) {
			w := strings.Builder{}
			actualArgs := FixDeprecatedFlags(strings.Split(tc.cli, " "), &w)

			assert.Equal(t, tc.expectArgs, actualArgs)
		})
	}
}

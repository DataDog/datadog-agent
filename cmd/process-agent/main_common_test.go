// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

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
				"--cfgpath",
				"datadog.yaml",
			},
			expectWarn: deprecatedFlagWarning("-config", "--cfgpath"),
		},
		{
			cli: "-config=datadog.yaml",
			expectArgs: []string{
				"--cfgpath=datadog.yaml",
			},
			expectWarn: deprecatedFlagWarning("-config", "--cfgpath"),
		},
		{
			cli: "--cfgpath=datadog.yaml",
			expectArgs: []string{
				"--cfgpath=datadog.yaml",
			},
		},
		{
			cli: "-sysprobe-config system-probe.yaml",
			expectArgs: []string{
				"--sysprobe-config",
				"system-probe.yaml",
			},
			expectWarn: deprecatedFlagWarning("-sysprobe-config", "--sysprobe-config"),
		},
		{
			cli: "-pid pidfile",
			expectArgs: []string{
				"--pid",
				"pidfile",
			},
			expectWarn: deprecatedFlagWarning("-pid", "--pid"),
		},
		{
			cli: "-info",
			expectArgs: []string{
				"--info",
			},
			expectWarn: deprecatedFlagWarning("-info", "--info"),
		},
		{
			cli: "-version",
			expectArgs: []string{
				"--version",
			},
			expectWarn: deprecatedFlagWarning("-version", "--version"),
		},
		{
			cli: "-check process",
			expectArgs: []string{
				"--check",
				"process",
			},
			expectWarn: deprecatedFlagWarning("-check", "--check"),
		},
		{
			cli: "-install-service",
			expectArgs: []string{
				"--install-service",
			},
			expectWarn: deprecatedFlagWarning("-install-service", "--install-service"),
		},
		{
			cli: "-uninstall-service",
			expectArgs: []string{
				"--uninstall-service",
			},
			expectWarn: deprecatedFlagWarning("-uninstall-service", "--uninstall-service"),
		},
		{
			cli: "-start-service",
			expectArgs: []string{
				"--start-service",
			},
			expectWarn: deprecatedFlagWarning("-start-service", "--start-service"),
		},
		{
			cli: "-stop-service",
			expectArgs: []string{
				"--stop-service",
			},
			expectWarn: deprecatedFlagWarning("-stop-service", "--stop-service"),
		},
		{
			cli: "-foreground",
			expectArgs: []string{
				"--foreground",
			},
			expectWarn: deprecatedFlagWarning("-foreground", "--foreground"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.cli, func(t *testing.T) {
			w := strings.Builder{}
			args := strings.Split(tc.cli, " ")
			fixDeprecatedFlags(args, &w)

			assert.Equal(t, tc.expectArgs, args)
			assert.Equal(t, tc.expectWarn, w.String())
		})
	}
}

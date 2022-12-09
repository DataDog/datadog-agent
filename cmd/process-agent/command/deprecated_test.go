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

func TestReplaceFlag(t *testing.T) {
	tests := []struct {
		name              string
		arg               string
		flag              string
		replacer          replaceFlagFunc
		expectReplaceFlag replaceFlag
	}{
		{
			name:     "replace posix no value",
			arg:      "-foo",
			flag:     "-foo",
			replacer: replaceFlagPosix,
			expectReplaceFlag: replaceFlag{
				hint: "--foo",
				args: []string{"--foo"},
			},
		},
		{
			name:     "replace posix with value single arg",
			arg:      "-foo=bar",
			flag:     "-foo",
			replacer: replaceFlagPosix,
			expectReplaceFlag: replaceFlag{
				hint: "--foo",
				args: []string{"--foo=bar"},
			},
		},
		{
			name:     "replace flag exact",
			arg:      "-version",
			flag:     "-version",
			replacer: replaceFlagExact("version"),
			expectReplaceFlag: replaceFlag{
				hint: "version",
				args: []string{"version"},
			},
		},
		{
			name:     "replace flag subcommand positional",
			arg:      "-check",
			flag:     "-check",
			replacer: replaceFlagSubCommandPosArg("check"),
			expectReplaceFlag: replaceFlag{
				hint: "check",
				args: []string{"check"},
			},
		},
		{
			name:     "replace flag subcommand positional with value",
			arg:      "-check=process",
			flag:     "-check",
			replacer: replaceFlagSubCommandPosArg("check"),
			expectReplaceFlag: replaceFlag{
				hint: "check",
				args: []string{"check", "process"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actualReplaceFlag := tc.replacer(tc.arg, tc.flag)
			assert.Equal(t, tc.expectReplaceFlag, actualReplaceFlag)
		})
	}
}

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
				"version",
			},
			expectWarn: deprecatedFlagWarning("-version", "version"),
		},
		{
			cli: "-check process",
			expectArgs: []string{
				"check",
				"process",
			},
			expectWarn: deprecatedFlagWarning("-check", "check"),
		},
		{
			cli: "--check process",
			expectArgs: []string{
				"check",
				"process",
			},
			expectWarn: deprecatedFlagWarning("--check", "check"),
		},
		{
			cli: "-check=process",
			expectArgs: []string{
				"check",
				"process",
			},
			expectWarn: deprecatedFlagWarning("-check", "check"),
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
			actualArgs := FixDeprecatedFlags(strings.Split(tc.cli, " "), &w)

			assert.Equal(t, tc.expectArgs, actualArgs)
			assert.Equal(t, tc.expectWarn, w.String())
		})
	}
}

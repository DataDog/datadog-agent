// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceFlag(t *testing.T) {
	tests := []struct {
		name              string
		arg               string
		flag              string
		replacer          ReplaceFlagFunc
		expectReplaceFlag ReplaceFlag
	}{
		{
			name:     "replace posix no value",
			arg:      "-foo",
			flag:     "-foo",
			replacer: ReplaceFlagPosix,
			expectReplaceFlag: ReplaceFlag{
				Hint: "--foo",
				Args: []string{"--foo"},
			},
		},
		{
			name:     "replace posix with value single arg",
			arg:      "-foo=bar",
			flag:     "-foo",
			replacer: ReplaceFlagPosix,
			expectReplaceFlag: ReplaceFlag{
				Hint: "--foo",
				Args: []string{"--foo=bar"},
			},
		},
		{
			name:     "replace flag exact",
			arg:      "-version",
			flag:     "-version",
			replacer: ReplaceFlagExact("version"),
			expectReplaceFlag: ReplaceFlag{
				Hint: "version",
				Args: []string{"version"},
			},
		},
		{
			name:     "replace flag subcommand positional",
			arg:      "-bar",
			flag:     "-bar",
			replacer: ReplaceFlagSubCommandPosArg("bar"),
			expectReplaceFlag: ReplaceFlag{
				Hint: "bar",
				Args: []string{"bar"},
			},
		},
		{
			name:     "replace flag subcommand positional with value",
			arg:      "-foo=bar",
			flag:     "-foo",
			replacer: ReplaceFlagSubCommandPosArg("foo"),
			expectReplaceFlag: ReplaceFlag{
				Hint: "foo",
				Args: []string{"foo", "bar"},
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

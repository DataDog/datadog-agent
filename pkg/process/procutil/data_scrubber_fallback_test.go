// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package procutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripArguments(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected string
	}{
		{
			name:     "OS parse",
			cmdline:  []string{"agent", "-password", "1234"},
			expected: "agent",
		},
		{
			name:     "No OS parse",
			cmdline:  []string{"python ~/test/run.py -open_password=admin -consul_token 2345 -blocked_from_yamt=1234 &"},
			expected: "python",
		},
		{
			name:     "No OS parse + whitespace",
			cmdline:  []string{"java   -password      1234"},
			expected: "java",
		},
		{
			name:     "Optional dash args",
			cmdline:  []string{"agent password:1234"},
			expected: "agent",
		},
		{
			name:     "Single dash args",
			cmdline:  []string{"agent -password:1234"},
			expected: "agent",
		},
		{
			name:     "Double dash args",
			cmdline:  []string{"agent --password:1234"},
			expected: "agent",
		},
	} {
		scrubber := setupDataScrubber(t)
		scrubber.StripAllArguments = true

		t.Run(tc.name, func(t *testing.T) {
			actual := scrubber.stripArguments(tc.cmdline)
			assert.Equal(t, actual[0], tc.expected)
		})
	}
}

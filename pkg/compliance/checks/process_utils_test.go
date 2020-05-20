// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCmdLine(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name: "Mixed flags",
			args: []string{"str", "--path=foo", "--foo", "bar", "-baz", "42", "--activate", "--verbose", "-f"},
			expected: map[string]string{
				"str":        "",
				"--path":     "foo",
				"--foo":      "bar",
				"-baz":       "42",
				"--activate": "",
				"--verbose":  "",
				"-f":         "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseProcessCmdLine(tt.args)
			assert.Equal(t, tt.expected, parsed)
		})
	}
}

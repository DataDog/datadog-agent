// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package manager

import (
	"regexp"
	"testing"

	assert "github.com/stretchr/testify/require"
)

type processFixture struct {
	name string

	processes  processes
	regexps    []*regexp.Regexp
	shouldExit bool
}

func (f *processFixture) run(t *testing.T) {
	t.Helper()

	processFetcher = func() (processes, error) {
		for pid, p := range f.processes {
			p.Pid = pid
		}
		return f.processes, nil
	}

	c := NoProcessExit(f.regexps)
	assert.Equal(t, f.shouldExit, c.check())
}

func TestExitDetection(t *testing.T) {
	tests := []processFixture{
		{
			name: "existing process",
			processes: processes{
				42: {
					Name: "agent",
				},
				100: {
					Name: "foo",
				},
				101: {
					Name: "pause",
				},
				102: {
					Name: "security-agent",
				},
			},
			regexps:    defaultRegexps,
			shouldExit: false,
		},
		{
			name: "no other case",
			processes: processes{
				42: {
					Name: "agent",
				},
				101: {
					Name: "pause",
				},
				102: {
					Name: "security-agent",
				},
			},
			regexps:    defaultRegexps,
			shouldExit: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

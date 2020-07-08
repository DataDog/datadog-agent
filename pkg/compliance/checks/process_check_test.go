// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	"github.com/DataDog/gopsutil/process"

	"github.com/stretchr/testify/assert"
)

type processFixture struct {
	name    string
	process *compliance.Process

	processes map[int32]*process.FilledProcess
	expKV     compliance.KVMap
	expError  error
}

func (f *processFixture) run(t *testing.T) {
	t.Helper()

	cache.Cache.Delete(processCacheKey)
	processFetcher = func() (map[int32]*process.FilledProcess, error) {
		return f.processes, nil
	}

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	if f.expKV != nil {
		env.On("Reporter").Return(reporter)
		reporter.On(
			"Report",
			newTestRuleEvent(
				[]string{"check_kind:process"},
				f.expKV,
			),
		).Once()
	}

	check, err := newProcessCheck(newTestBaseCheck(env, checkKindProcess), f.process)
	assert.NoError(t, err)

	err = check.Run()
	assert.Equal(t, f.expError, err)
}

func TestProcessCheck(t *testing.T) {
	tests := []processFixture{
		{
			name: "Simple case",
			process: &compliance.Process{
				Name: "proc1",
				Report: compliance.Report{
					{
						Kind:     "flag",
						Property: "--path",
						As:       "path",
					},
				},
			},
			processes: map[int32]*process.FilledProcess{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--path=foo"},
				},
			},
			expKV: compliance.KVMap{
				"path": "foo",
			},
		},
		{
			name: "Process not found",
			process: &compliance.Process{
				Name: "proc1",
				Report: compliance.Report{
					{
						Kind:     "flag",
						Property: "--path",
						As:       "path",
					},
				},
			},
			processes: map[int32]*process.FilledProcess{
				42: {
					Name:    "proc2",
					Cmdline: []string{"arg1", "--path=foo"},
				},
				43: {
					Name:    "proc3",
					Cmdline: []string{"arg1", "--path=foo"},
				},
			},
			expKV: nil,
		},
		{
			name: "Argument not found",
			process: &compliance.Process{
				Name: "proc1",
				Report: compliance.Report{
					{
						Kind:     "flag",
						Property: "--path",
						As:       "path",
					},
				},
			},
			processes: map[int32]*process.FilledProcess{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--paths=foo"},
				},
			},
			expKV: nil,
		},
		{
			name: "Override returned value",
			process: &compliance.Process{
				Name: "proc1",
				Report: compliance.Report{
					{
						Kind:     "flag",
						Property: "--verbose",
						As:       "verbose",
						Value:    "true",
					},
				},
			},
			processes: map[int32]*process.FilledProcess{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--verbose"},
				},
			},
			expKV: compliance.KVMap{
				"verbose": "true",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

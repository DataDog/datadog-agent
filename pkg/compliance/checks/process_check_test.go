// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.
package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	"github.com/DataDog/gopsutil/process"

	assert "github.com/stretchr/testify/require"
)

type processFixture struct {
	name     string
	resource compliance.Resource

	processes    map[int32]*process.FilledProcess
	expectReport *report
	expectError  error
}

func (f *processFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	cache.Cache.Delete(processCacheKey)
	processFetcher = func() (map[int32]*process.FilledProcess, error) {
		return f.processes, nil
	}

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	expr, err := eval.ParseIterable(f.resource.Condition)
	assert.NoError(err)

	result, err := checkProcess(env, "rule-id", f.resource, expr)
	assert.Equal(f.expectReport, result)
	assert.Equal(f.expectError, err)

}

func TestProcessCheck(t *testing.T) {
	tests := []processFixture{
		{
			name: "simple case",
			resource: compliance.Resource{
				Process: &compliance.Process{
					Name: "proc1",
				},
				Condition: `process.flag("--path") == "foo"`,
			},
			processes: map[int32]*process.FilledProcess{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--path=foo"},
				},
			},
			expectReport: &report{
				passed: true,
				data: event.Data{
					"process.name":    "proc1",
					"process.exe":     "",
					"process.cmdLine": []string{"arg1", "--path=foo"},
				},
			},
		},
		{
			name: "process not found",
			resource: compliance.Resource{
				Process: &compliance.Process{
					Name: "proc1",
				},
				Condition: `process.flag("--path") == "foo"`,
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
			expectReport: &report{
				passed: false,
			},
		},
		{
			name: "argument not found",
			resource: compliance.Resource{
				Process: &compliance.Process{
					Name: "proc1",
				},
				Condition: `process.flag("--path") == "foo"`,
			},
			processes: map[int32]*process.FilledProcess{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--paths=foo"},
				},
			},
			expectReport: &report{
				passed: false,
				data: event.Data{
					"process.name":    "proc1",
					"process.exe":     "",
					"process.cmdLine": []string{"arg1", "--paths=foo"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

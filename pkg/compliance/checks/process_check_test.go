// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	assert "github.com/stretchr/testify/require"
)

type processFixture struct {
	name     string
	resource compliance.Resource

	processes    processes
	useCache     bool
	expectReport *compliance.Report
	expectError  error
}

func (f *processFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	if !f.useCache {
		cache.Cache.Delete(processCacheKey)
	}
	processFetcher = func() (processes, error) {
		return f.processes, nil
	}

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	processCheck, err := newResourceCheck(env, "rule-id", f.resource)
	assert.NoError(err)

	result, err := processCheck.check(env)
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
			processes: processes{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--path=foo"},
				},
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"process.name":    "proc1",
					"process.exe":     "",
					"process.cmdLine": []string{"arg1", "--path=foo"},
				},
			},
		},
		{
			name: "fallback case",
			resource: compliance.Resource{
				Process: &compliance.Process{
					Name: "proc1",
				},
				Condition: `process.flag("--tlsverify") != ""`,
				Fallback: &compliance.Fallback{
					Condition: `!process.hasFlag("--tlsverify")`,
					Resource: compliance.Resource{
						Process: &compliance.Process{
							Name: "proc2",
						},
						Condition: `process.hasFlag("--tlsverify")`,
					},
				},
			},
			processes: processes{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1"},
				},
				38: {
					Name:    "proc2",
					Cmdline: []string{"arg1", "--tlsverify"},
				},
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"process.name":    "proc2",
					"process.exe":     "",
					"process.cmdLine": []string{"arg1", "--tlsverify"},
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
			processes: processes{
				42: {
					Name:    "proc2",
					Cmdline: []string{"arg1", "--path=foo"},
				},
				43: {
					Name:    "proc3",
					Cmdline: []string{"arg1", "--path=foo"},
				},
			},
			expectReport: &compliance.Report{
				Passed: false,
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
			processes: processes{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--paths=foo"},
				},
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
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

func TestProcessCheckCache(t *testing.T) {
	// Run first fixture, populating cache
	firstContent := processFixture{
		name: "simple case",
		resource: compliance.Resource{
			Process: &compliance.Process{
				Name: "proc1",
			},
			Condition: `process.flag("--path") == "foo"`,
		},
		processes: processes{
			42: {
				Name:    "proc1",
				Cmdline: []string{"arg1", "--path=foo"},
			},
		},
		expectReport: &compliance.Report{
			Passed: true,
			Data: event.Data{
				"process.name":    "proc1",
				"process.exe":     "",
				"process.cmdLine": []string{"arg1", "--path=foo"},
			},
		},
	}
	firstContent.run(t)

	// Run second fixture, using cache
	secondFixture := processFixture{
		name: "simple case",
		resource: compliance.Resource{
			Process: &compliance.Process{
				Name: "proc1",
			},
			Condition: `process.flag("--path") == "foo"`,
		},
		useCache: true,
		expectReport: &compliance.Report{
			Passed: true,
			Data: event.Data{
				"process.name":    "proc1",
				"process.exe":     "",
				"process.cmdLine": []string{"arg1", "--path=foo"},
			},
		},
	}
	secondFixture.run(t)
}

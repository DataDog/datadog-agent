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

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

type regoFixture struct {
	name      string
	resources []compliance.RegoInput
	module    string
	findings  string
	scope     compliance.RuleScope

	processes     processes
	useCache      bool
	expectReports []*compliance.Report
	expectError   error
}

func (f *regoFixture) newRegoCheck() (*regoCheck, error) {
	ruleID := "rule-id"
	rule := &compliance.RegoRule{
		RuleCommon: compliance.RuleCommon{
			ID: ruleID,
		},
		Module:   f.module,
		Findings: f.findings,
	}

	regoCheck := &regoCheck{
		ruleID:    ruleID,
		resources: f.resources,
	}

	if err := regoCheck.compileRule(rule, f.scope, &compliance.SuiteMeta{}); err != nil {
		return nil, err
	}

	return regoCheck, nil
}

func (f *regoFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	if !f.useCache {
		cache.Cache.Delete(processCacheKey)
	}
	processFetcher = func() (processes, error) {
		for pid, p := range f.processes {
			p.Pid = pid
		}
		return f.processes, nil
	}

	env := &mocks.Env{}
	env.On("MaxEventsPerRun").Return(30).Maybe()
	env.On("ProvidedInput", mock.Anything).Return(nil).Once()
	env.On("Hostname").Return("hostname_test").Once()
	env.On("DumpInputPath").Return("").Once()

	defer env.AssertExpectations(t)

	regoCheck, err := f.newRegoCheck()
	assert.NoError(err)

	reports := regoCheck.check(env)
	assert.Equal(f.expectReports, reports)
	assert.Equal(f.expectError, reports[0].Error)
}

func TestRegoProcessCheck(t *testing.T) {
	tests := []regoFixture{
		{
			name: "simple case",
			resources: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc1",
						},
					},
					TagName: "processes",
				},
			},
			module: `
				package test

				import data.datadog as dd

				default valid = false

				findings[f] {
					p := input.processes[_]
					p.flags["--path"] == "foo"
					f := dd.passed_finding("process", "42", dd.process_data(p))
				}
			`,
			findings: "data.test.findings",
			processes: processes{
				42: {
					Name:    "proc1",
					Cmdline: []string{"arg1", "--path=foo"},
				},
			},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"process.name":    "proc1",
						"process.exe":     "",
						"process.cmdLine": []interface{}{"arg1", "--path=foo"},
					},
					Resource: compliance.ReportResource{
						ID:   "42",
						Type: "process",
					},
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

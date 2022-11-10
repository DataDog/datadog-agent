// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
)

type regoFixture struct {
	name     string
	inputs   []compliance.RegoInput
	module   string
	findings string

	processes     processes
	expectReports []*compliance.Report
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
		ruleID: ruleID,
		inputs: f.inputs,
	}

	if err := regoCheck.compileRule(rule, nil, &compliance.SuiteMeta{}); err != nil {
		return nil, err
	}

	return regoCheck, nil
}

func (f *regoFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	cache.Cache.Delete(processCacheKey)
	processFetcher = func() (processes, error) {
		return f.processes, nil
	}

	env := &mocks.Env{}
	env.On("MaxEventsPerRun").Return(30).Maybe()
	env.On("ProvidedInput", mock.Anything).Return(nil).Once()
	env.On("Hostname").Return("hostname_test").Once()
	env.On("DumpInputPath").Return("").Once()
	env.On("ShouldSkipRegoEval").Return(false).Once()

	defer env.AssertExpectations(t)

	regoCheck, err := f.newRegoCheck()
	assert.NoError(err)

	reports := regoCheck.check(env)
	assert.Equal(f.expectReports, reports)
}

func TestRegoCheck(t *testing.T) {
	tests := []regoFixture{
		{
			name: "simple case",
			inputs: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc1",
						},
					},
					TagName: "processes",
					Type:    "array",
				},
			},
			module: `
				package test

				import data.datadog as dd

				process_data(p) = d {
					d := {
						"process.name": p.name,
						"process.exe": p.exe,
						"process.cmdLine": p.cmdLine,
					}
				}

				default valid = false

				findings[f] {
					p := input.processes[_]
					p.flags["--path"] == "foo"
					f := dd.passed_finding("process", "42", process_data(p))
				}
			`,
			findings: "data.test.findings",
			processes: processes{
				NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
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
					Evaluator: "rego",
				},
			},
		},
		{
			name: "status normalization",
			inputs: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc1",
						},
					},
					TagName: "processes",
					Type:    "array",
				},
			},
			module: `
				package test

				import data.datadog as dd

				process_data(p) = d {
					d := {
						"process.name": p.name,
						"process.exe": p.exe,
						"process.cmdLine": p.cmdLine,
					}
				}

				default valid = false

				findings[f] {
					p := input.processes[_]
					p.flags["--path"] == "foo"
					f := {
						"status": "pass",
						"resource_type": "process",
						"resource_id": "42",
						"data": process_data(p),
					}
				}
			`,
			findings: "data.test.findings",
			processes: processes{
				NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
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
					Evaluator: "rego",
				},
			},
		},
		{
			name: "failing case",
			inputs: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc1",
						},
					},
					TagName: "processes",
					Type:    "array",
				},
			},
			module: `
				package test

				import data.datadog as dd

				process_data(p) = d {
					d := {
						"process.name": p.name,
						"process.exe": p.exe,
						"process.cmdLine": p.cmdLine,
					}
				}

				default valid = false

				findings[f] {
					p := input.processes[_]
					p.flags["--path"] == "foo"
					f := dd.failing_finding("process", "42", process_data(p))
				}
			`,
			findings: "data.test.findings",
			processes: processes{
				NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
			},
			expectReports: []*compliance.Report{
				{
					Passed: false,
					Data: event.Data{
						"process.name":    "proc1",
						"process.exe":     "",
						"process.cmdLine": []interface{}{"arg1", "--path=foo"},
					},
					Resource: compliance.ReportResource{
						ID:   "42",
						Type: "process",
					},
					Evaluator: "rego",
				},
			},
		},
		{
			name: "error case",
			inputs: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc1",
						},
					},
					TagName: "processes",
					Type:    "array",
				},
			},
			module: `
				package test

				import data.datadog as dd

				default valid = false

				findings[f] {
					p := input.processes[_]
					f := dd.error_finding("process", "42", "error message")
				}
			`,
			findings: "data.test.findings",
			processes: processes{
				NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
			},
			expectReports: []*compliance.Report{
				{
					Passed: false,
					Data:   nil,
					Resource: compliance.ReportResource{
						ID:   "42",
						Type: "process",
					},
					Evaluator:         "rego",
					Error:             errors.New("error message"),
					UserProvidedError: true,
				},
			},
		},
		{
			name: "empty case",
			inputs: []compliance.RegoInput{
				{
					ResourceCommon: compliance.ResourceCommon{
						Process: &compliance.Process{
							Name: "proc2",
						},
					},
					TagName: "processes",
					Type:    "array",
				},
			},
			module: `
				package test

				import data.datadog as dd

				default valid = false

				findings[f] {
					p := input.processes[_]
					f := dd.error_finding("process", "42", "error message")
				}
			`,
			findings: "data.test.findings",
			processes: processes{
				NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
			},
			expectReports: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

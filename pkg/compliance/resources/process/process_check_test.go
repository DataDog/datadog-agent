// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	resource_test "github.com/DataDog/datadog-agent/pkg/compliance/resources/tests"
	processutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/process"
	"github.com/DataDog/datadog-agent/pkg/util/cache"

	assert "github.com/stretchr/testify/require"
)

var processModule = `package datadog
			
import data.datadog as dd
import data.helpers as h

compliant(process) {
	%s
}

findings[f] {
	count(input.process) == 0
	f := dd.failing_finding(
			"process",
			"",
			null,
	)
}

findings[f] {
		compliant(input.process)
		f := dd.passed_finding(
				"process",
				input.process.pid,
				{ "process.exe": input.process.exe,
				  "process.name": input.process.name,
				  "process.cmdLine": input.process.cmdLine,
				  "process.pid": input.process.pid }
		)
}

findings[f] {
		not compliant(input.process)
		f := dd.failing_finding(
				"process",
				input.process.pid,
				{ "process.exe": input.process.exe,
				  "process.name": input.process.name,
				  "process.cmdLine": input.process.cmdLine,
				  "process.pid": input.process.pid }
		)
}`

type processFixture struct {
	name     string
	module   string
	resource compliance.RegoInput

	processes    processutils.Processes
	useCache     bool
	expectReport *compliance.Report
	expectError  error
}

func (f *processFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	if !f.useCache {
		cache.Cache.Delete(processutils.ProcessCacheKey)
	}
	processutils.Fetcher = func() (processutils.Processes, error) {
		return f.processes, nil
	}

	env := &mocks.Env{}
	env.On("MaxEventsPerRun").Return(30).Maybe()
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()

	defer env.AssertExpectations(t)

	regoRule := resource_test.NewTestRule(f.resource, "group", f.module)

	processCheck := rego.NewCheck(regoRule)
	err := processCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := processCheck.Check(env)

	assert.Equal(f.expectReport, reports[0])
	assert.Equal(f.expectError, reports[0].Error)
}

func TestProcessCheck(t *testing.T) {
	tests := []processFixture{
		{
			name: "simple case",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Process: &compliance.Process{
						Name: "proc1",
					},
				},
				Type: "object",
				// Condition: `process.flag("--path") == "foo"`,
			},
			module: fmt.Sprintf(processModule, `process.flags["--path"] == "foo"`),
			processes: processutils.Processes{
				42: processutils.NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"process.name":    "proc1",
					"process.exe":     "",
					"process.cmdLine": []interface{}{"arg1", "--path=foo"},
					"process.pid":     json.Number("42"),
				},
				Resource: compliance.ReportResource{
					ID:   "42",
					Type: "process",
				},
				Evaluator: "rego",
			},
		},
		{
			name: "process not found",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Process: &compliance.Process{
						Name: "proc1",
					},
				},
				Type: "object",
				// Condition: `process.flag("--path") == "foo"`,
			},
			module: fmt.Sprintf(processModule, `process.flags["--path"] == "foo"`),
			processes: processutils.Processes{
				42: processutils.NewCheckedFakeProcess(42, "proc2", []string{"arg1", "--path=foo"}),
				43: processutils.NewCheckedFakeProcess(43, "proc3", []string{"arg1", "--path=foo"}),
			},
			expectReport: &compliance.Report{
				Passed:    false,
				Evaluator: "rego",
				Resource: compliance.ReportResource{
					ID:   "",
					Type: "process",
				},
			},
		},
		{
			name: "argument not found",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Process: &compliance.Process{
						Name: "proc1",
					},
				},
				Type: "object",
				// Condition: `process.flag("--path") == "foo"`,
			},
			module: fmt.Sprintf(processModule, `process.flags["--path"] == "foo"`),
			processes: processutils.Processes{
				42: processutils.NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--paths=foo"}),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					"process.name":    "proc1",
					"process.exe":     "",
					"process.cmdLine": []interface{}{"arg1", "--paths=foo"},
					"process.pid":     json.Number("42"),
				},
				Resource: compliance.ReportResource{
					ID:   "42",
					Type: "process",
				},
				Evaluator: "rego",
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
		resource: compliance.RegoInput{
			ResourceCommon: compliance.ResourceCommon{
				Process: &compliance.Process{
					Name: "proc1",
				},
			},
			Type: "object",
			// Condition: `process.flag("--path") == "foo"`,
		},
		module: fmt.Sprintf(processModule, `process.flags["--path"] == "foo"`),
		processes: processutils.Processes{
			42: processutils.NewCheckedFakeProcess(42, "proc1", []string{"arg1", "--path=foo"}),
		},
		expectReport: &compliance.Report{
			Passed: true,
			Data: event.Data{
				"process.name":    "proc1",
				"process.exe":     "",
				"process.cmdLine": []interface{}{"arg1", "--path=foo"},
				"process.pid":     json.Number("42"),
			},
			Resource: compliance.ReportResource{
				ID:   "42",
				Type: "process",
			},
			Evaluator: "rego",
		},
	}
	firstContent.run(t)

	// Run second fixture, using cache
	secondFixture := processFixture{
		name: "simple case",
		resource: compliance.RegoInput{
			ResourceCommon: compliance.ResourceCommon{
				Process: &compliance.Process{
					Name: "proc1",
				},
			},
			Type: "object",
			// Condition: `process.flag("--path") == "foo"`,
		},
		module:   fmt.Sprintf(processModule, `process.flags["--path"] == "foo"`),
		useCache: true,
		expectReport: &compliance.Report{
			Passed: true,
			Data: event.Data{
				"process.name":    "proc1",
				"process.exe":     "",
				"process.cmdLine": []interface{}{"arg1", "--path=foo"},
				"process.pid":     json.Number("42"),
			},
			Resource: compliance.ReportResource{
				ID:   "42",
				Type: "process",
			},
			Evaluator: "rego",
		},
	}
	secondFixture.run(t)
}

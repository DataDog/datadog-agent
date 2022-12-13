// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package xccdf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	resource_test "github.com/DataDog/datadog-agent/pkg/compliance/resources/tests"

	assert "github.com/stretchr/testify/require"
)

var xccdfModule = `package datadog

import data.datadog as dd

passed_result(result) {
        result.result == "passed"
}

failing_result(result) {
        result.result == "failing"
}

findings[f] {
        xccdf_result := input.xccdf[_]
        passed_result(xccdf_result)
        f := dd.passed_finding(
                "host",
                xccdf_result.name,
                {},
        )
}

findings[f] {
        xccdf_result := input.xccdf[_]
        failing_result(xccdf_result)
        f := dd.failing_finding(
                "host",
                xccdf_result.name,
                {},
        )
}`

type xccdfFixture struct {
	name     string
	module   string
	resource compliance.RegoInput

	expectReport *compliance.Report
	expectError  error
}

func (f *xccdfFixture) run(t *testing.T) {
	t.Helper()
	assert := assert.New(t)

	env := &mocks.Env{}
	env.On("MaxEventsPerRun").Return(30).Maybe()
	env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
	env.On("DumpInputPath").Return("").Maybe()
	env.On("ShouldSkipRegoEval").Return(false).Maybe()
	env.On("Hostname").Return("test-host").Maybe()
	env.On("ConfigDir").Return("testdata/")

	defer env.AssertExpectations(t)

	regoRule := resource_test.NewTestRule(f.resource, "xccdf", f.module)

	xccdfCheck := rego.NewCheck(regoRule)
	err := xccdfCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
	assert.NoError(err)

	reports := xccdfCheck.Check(env)

	assert.NotEmpty(reports)
	assert.Equal(f.expectReport, reports[0])
	assert.Equal(f.expectError, reports[0].Error)
}

func TestXccdbCheck(t *testing.T) {
	tests := []xccdfFixture{
		{
			name: "simple case",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Xccdf: &compliance.Xccdf{
						Cpe:     "ssg-ubuntu2204-cpe-dictionary.xml",
						Rule:    "xccdf_org.ssgproject.content_rule_package_aide_installed",
						Name:    "ssg-ubuntu2204-xccdf.xml",
						Profile: "xccdf_org.ssgproject.content_profile_cis_level1_server",
					},
				},
			},
			module: xccdfModule,
			expectReport: &compliance.Report{
				Data:   event.Data{},
				Passed: true,
				Resource: compliance.ReportResource{
					ID:   "xccdf_org.ssgproject.content_rule_package_aide_installed",
					Type: "host",
				},
				Evaluator: "rego",
				// Error:             errors.New("Not applicable"),
				// UserProvidedError: true,
			},
			// expectError: errors.New("Not applicable"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

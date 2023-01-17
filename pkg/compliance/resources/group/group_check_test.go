// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package group

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/compliance/rego"
	_ "github.com/DataDog/datadog-agent/pkg/compliance/resources/constants"
	resource_test "github.com/DataDog/datadog-agent/pkg/compliance/resources/tests"

	assert "github.com/stretchr/testify/require"
)

func TestGroupCheck(t *testing.T) {
	module := `package datadog

	import data.datadog as dd
	import data.helpers as h
	
	user_in_group(group) {
		count([u | u := group.users[_]; u == "%s"]) > 0
	}
	
	findings[f] {
		user_in_group(input.group)
		f := dd.passed_finding(
				h.resource_type,
				input.group.name,
				h.group_data(input.group),
		)
	}

	findings[f] {
		not user_in_group(input.group)
		f := dd.failing_finding(
				h.resource_type,
				input.group.name,
				h.group_data(input.group),
		)
	}
	`

	tests := []struct {
		name         string
		etcGroupFile string
		resource     compliance.RegoInput
		module       string

		expectReport *compliance.Report
		expectError  error
	}{
		{
			name:         "docker group user found",
			etcGroupFile: "./testdata/etc-group",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Group: &compliance.Group{
						Name: "docker",
					},
				},
				Type: "object",
			},
			module: fmt.Sprintf(module, "carlos"),

			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"group.name":  "docker",
					"group.id":    json.Number("412"),
					"group.users": []interface{}{"alice", "bob", "carlos", "dan", "eve"},
				},
				Resource: compliance.ReportResource{
					ID:   "docker",
					Type: "group",
				},
				Evaluator: "rego",
			},
		},
		{
			name:         "docker group user not found",
			etcGroupFile: "./testdata/etc-group",
			resource: compliance.RegoInput{
				ResourceCommon: compliance.ResourceCommon{
					Group: &compliance.Group{
						Name: "docker",
					},
				},
				Type: "object",
			},
			module: fmt.Sprintf(module, "carol"),

			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					"group.name":  "docker",
					"group.id":    json.Number("412"),
					"group.users": []interface{}{"alice", "bob", "carlos", "dan", "eve"},
				},
				Resource: compliance.ReportResource{
					ID:   "docker",
					Type: "group",
				},
				Evaluator: "rego",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			env := &mocks.Env{}
			env.On("EtcGroupPath").Return(test.etcGroupFile)
			env.On("ProvidedInput", "rule-id").Return(nil).Maybe()
			env.On("DumpInputPath").Return("").Maybe()
			env.On("ShouldSkipRegoEval").Return(false).Maybe()
			env.On("Hostname").Return("test-host").Maybe()

			regoRule := resource_test.NewTestRule(test.resource, "group", test.module)

			dockerCheck := rego.NewCheck(regoRule)
			err := dockerCheck.CompileRule(regoRule, "", &compliance.SuiteMeta{}, nil)
			assert.NoError(err)

			reports := dockerCheck.Check(env)

			assert.Equal(test.expectReport, reports[0])
			assert.Equal(test.expectError, reports[0].Error)
		})
	}
}

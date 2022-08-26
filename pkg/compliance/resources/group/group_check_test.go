// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package group

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"
)

func TestGroupCheck(t *testing.T) {
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
			},
			module: `package datadog

import data.datadog as dd
import data.helpers as h

carlos_in_group(group) {
	[u | u := group.users[_]; u == "carlos"])
}

findings[f] {
	carlos_in_group(input.group)
	f := dd.passed_finding(
			h.resource_type,
			h.resource_id,
			h.group_data(input.group),
	)
}
`,

			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"group.name":  "docker",
					"group.id":    412,
					"group.users": []string{"alice", "bob", "carlos", "dan", "eve"},
				},
				Resource: compliance.ReportResource{
					ID:   "docker",
					Type: "group",
				},
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
			},
			module: "",

			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					"group.name":  "docker",
					"group.id":    412,
					"group.users": []string{"alice", "bob", "carlos", "dan", "eve"},
				},
				Resource: compliance.ReportResource{
					ID:   "docker",
					Type: "group",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			env := &mocks.Env{}
			env.On("EtcGroupPath").Return(test.etcGroupFile)

			groupCheck, err := Resolve(context.Background(), env, "rule-id", test.resource.ResourceCommon, true)
			assert.NoError(err)

			// conditionExpression, _ := eval.Cache.ParseIterable("_")

			reports := groupCheck.Evaluate(conditionExpression, env)
			assert.Equal(test.expectReport, reports[0])
			assert.Equal(test.expectError, reports[0].Error)
		})
	}
}

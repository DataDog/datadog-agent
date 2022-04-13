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

	assert "github.com/stretchr/testify/require"
)

func TestGroupCheck(t *testing.T) {
	tests := []struct {
		name         string
		etcGroupFile string
		resource     compliance.Resource

		expectReport *compliance.Report
		expectError  error
	}{
		{
			name:         "docker group user found",
			etcGroupFile: "./testdata/group/etc-group",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Group: &compliance.Group{
						Name: "docker",
					},
				},
				Condition: `"carlos" in group.users`,
			},

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
			etcGroupFile: "./testdata/group/etc-group",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Group: &compliance.Group{
						Name: "docker",
					},
				},
				Condition: `"carol" in group.users`,
			},

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

			groupCheck, err := newResourceCheck(env, "rule-id", test.resource)
			assert.NoError(err)

			reports := groupCheck.check(env)
			assert.Equal(test.expectReport, reports[0])
			assert.Equal(test.expectError, reports[0].Error)
		})
	}
}

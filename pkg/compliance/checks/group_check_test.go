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

	assert "github.com/stretchr/testify/require"
)

func TestGroupCheck(t *testing.T) {
	tests := []struct {
		name         string
		etcGroupFile string
		resource     compliance.Resource

		expectReport *report
		expectError  error
	}{
		{
			name:         "docker group user found",
			etcGroupFile: "./testdata/group/etc-group",
			resource: compliance.Resource{
				Group: &compliance.Group{
					Name: "docker",
				},
				Condition: `"carlos" in group.users`,
			},

			expectReport: &report{
				passed: true,
				data: event.Data{
					"group.name":  "docker",
					"group.id":    412,
					"group.users": []string{"alice", "bob", "carlos", "dan", "eve"},
				},
			},
		},
		{
			name:         "docker group user not found",
			etcGroupFile: "./testdata/group/etc-group",
			resource: compliance.Resource{
				Group: &compliance.Group{
					Name: "docker",
				},
				Condition: `"carol" in group.users`,
			},

			expectReport: &report{
				passed: false,
				data: event.Data{
					"group.name":  "docker",
					"group.id":    412,
					"group.users": []string{"alice", "bob", "carlos", "dan", "eve"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			env := &mocks.Env{}
			env.On("EtcGroupPath").Return(test.etcGroupFile)

			expr, err := eval.ParseIterable(test.resource.Condition)
			assert.NoError(err)

			result, err := checkGroup(env, "rule-id", test.resource, expr)
			assert.Equal(test.expectReport, result)
			assert.Equal(test.expectError, err)
		})
	}
}

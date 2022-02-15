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
	assert "github.com/stretchr/testify/require"
)

func TestCheckableList(t *testing.T) {
	assert := assert.New(t)

	type outcome struct {
		reports []*compliance.Report
	}

	tests := []struct {
		name     string
		list     []outcome
		expected outcome
	}{
		{
			name: "first=passed, second=passed, third=pass => result=[all passed]",
			list: []outcome{
				{
					reports: []*compliance.Report{
						{
							Passed: true,
							Data: event.Data{
								"something": "passed",
							},
						},
					},
				},
				{
					reports: []*compliance.Report{
						{
							Passed: true,
							Data: event.Data{
								"something else": "passed",
							},
						},
					},
				},
				{
					reports: []*compliance.Report{
						{
							Passed: true,
							Data: event.Data{
								"something else": "failed",
							},
						},
					},
				},
			},
			expected: outcome{
				reports: []*compliance.Report{
					{
						Passed: true,
						Data: event.Data{
							"something": "passed",
						},
					},
					{
						Passed: true,
						Data: event.Data{
							"something else": "passed",
						},
					},
					{
						Passed: false,
						Data: event.Data{
							"truncated": 1,
						},
						Error: errors.New("truncated result"),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := &mocks.Env{}
			env.On("MaxEventsPerRun").Return(2)

			var list checkableList

			for _, outcome := range test.list {
				c := &mockCheckable{}
				c.On("check", env).Return(outcome.reports)
				list = append(list, c)
			}

			reports := list.check(env)
			assert.Equal(test.expected.reports, reports)
		})
	}
}

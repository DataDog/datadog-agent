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
		report *compliance.Report
		err    error
	}

	tests := []struct {
		name     string
		list     []outcome
		expected outcome
	}{
		{
			name: "first=passed, second=failed => result=[failed from second]",
			list: []outcome{
				{
					report: &compliance.Report{
						Passed: true,
						Data: event.Data{
							"something": "passed",
						},
					},
				},
				{
					report: &compliance.Report{
						Passed: false,
						Data: event.Data{
							"something": "failed",
						},
					},
				},
			},
			expected: outcome{
				report: &compliance.Report{
					Passed: false,
					Data: event.Data{
						"something": "failed",
					},
				},
			},
		},
		{
			name: "first=error, second=passed => result[error from first]",
			list: []outcome{
				{
					err: errors.New("some error"),
				},
				{
					report: &compliance.Report{
						Passed: true,
						Data: event.Data{
							"something else": "passed",
						},
					},
				},
			},
			expected: outcome{
				err: errors.New("some error"),
			},
		},
		{
			name: "first=failed, second=passed => result[failed from first]",
			list: []outcome{
				{
					report: &compliance.Report{
						Passed: false,
						Data: event.Data{
							"something": "failed",
						},
					},
				},
				{
					report: &compliance.Report{
						Passed: true,
						Data: event.Data{
							"something else": "passed",
						},
					},
				},
			},
			expected: outcome{
				report: &compliance.Report{
					Passed: false,
					Data: event.Data{
						"something": "failed",
					},
				},
			},
		},
		{
			name: "first=failed, second=error => result[failed from first]",
			list: []outcome{
				{
					report: &compliance.Report{
						Passed: false,
						Data: event.Data{
							"something": "failed",
						},
					},
				},
				{
					err: errors.New("some other error"),
				},
			},
			expected: outcome{
				report: &compliance.Report{
					Passed: false,
					Data: event.Data{
						"something": "failed",
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := &mocks.Env{}

			var list checkableList

			for _, outcome := range test.list {
				c := &mockCheckable{}
				c.On("check", env).Return(outcome.report, outcome.err)
				list = append(list, c)
			}

			report, err := list.check(env)
			assert.Equal(test.expected.report, report)
			assert.Equal(test.expected.err, err)
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	assert "github.com/stretchr/testify/require"
)

func TestCheckableList(t *testing.T) {
	assert := assert.New(t)

	type outcome struct {
		report *report
		err    error
	}

	tests := []struct {
		name     string
		list     []outcome
		expected outcome
	}{
		{
			name: "first succeeds and is the result",
			list: []outcome{
				{
					report: &report{
						passed: true,
						data: event.Data{
							"something": "passed",
						},
					},
				},
				{
					report: &report{
						passed: false,
						data: event.Data{
							"something": "failed",
						},
					},
				},
			},
			expected: outcome{
				report: &report{
					passed: true,
					data: event.Data{
						"something": "passed",
					},
				},
			},
		},
		{
			name: "first is error and second succeeds and is the result",
			list: []outcome{
				{
					err: errors.New("some error"),
				},
				{
					report: &report{
						passed: true,
						data: event.Data{
							"something else": "passed",
						},
					},
				},
			},
			expected: outcome{
				report: &report{
					passed: true,
					data: event.Data{
						"something else": "passed",
					},
				},
			},
		},
		{
			name: "first not passed and second succeeds and is the result",
			list: []outcome{
				{
					report: &report{
						passed: false,
						data: event.Data{
							"something": "failed",
						},
					},
				},
				{
					report: &report{
						passed: true,
						data: event.Data{
							"something else": "passed",
						},
					},
				},
			},
			expected: outcome{
				report: &report{
					passed: true,
					data: event.Data{
						"something else": "passed",
					},
				},
			},
		},
		{
			name: "first not passed and second is error and is the result",
			list: []outcome{
				{
					report: &report{
						passed: false,
						data: event.Data{
							"something": "failed",
						},
					},
				},
				{
					err: errors.New("some other error"),
				},
			},
			expected: outcome{
				err: errors.New("some other error"),
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

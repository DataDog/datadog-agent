// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"
)

func TestResourceCheck(t *testing.T) {
	assert := assert.New(t)

	e := &mocks.Env{}
	e.On("MaxEventsPerRun").Return(30)

	fallbackReports := []*compliance.Report{
		{
			Passed: false,
			Data: event.Data{
				"fallback": true,
			},
		},
	}

	fallback := &mockCheckable{}
	fallback.On("check", e).Return(fallbackReports, nil)

	iterator := &mockIterator{els: []*eval.Instance{
		{
			Vars: map[string]interface{}{
				"a": 14,
			},
		},
		{
			Vars: map[string]interface{}{
				"a": 6,
			},
		},
		{
			Vars: map[string]interface{}{
				"a": 4,
			},
		},
	}}
	iterator.On("Next").Return()
	iterator.On("Done").Return()

	tests := []struct {
		name              string
		resourceCondition string
		resourceResolved  interface{}
		fallbackCondition string
		fallback          checkable
		reportedFields    []string

		expectReports []*compliance.Report
		expectErr     error
	}{
		{
			name:              "no fallback provided",
			resourceCondition: "a > 3",
			resourceResolved: &eval.Instance{
				Vars: map[string]interface{}{
					"a": 4,
					"b": 8,
				},
			},
			reportedFields: []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"a": 4,
					},
				},
			},
		},
		{
			name:              "fallback not used",
			resourceCondition: "a >= 3",
			resourceResolved: &eval.Instance{
				Vars: map[string]interface{}{
					"a": 4,
				},
			},
			fallbackCondition: "a == 3",
			fallback:          fallback,
			reportedFields:    []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"a": 4,
					},
				},
			},
		},
		{
			name:              "fallback used",
			resourceCondition: "a >= 3",
			resourceResolved: &eval.Instance{
				Vars: map[string]interface{}{
					"a": 3,
				},
			},
			fallbackCondition: "a == 3",
			fallback:          fallback,
			expectReports:     fallbackReports,
		},
		{
			name:              "cannot use fallback",
			resourceCondition: "a >= 3",
			resourceResolved: &instanceIterator{
				instances: []*eval.Instance{
					{
						Vars: map[string]interface{}{
							"a": 3,
						},
					},
				},
			},
			fallbackCondition: "a == 3",
			fallback:          fallback,
			expectReports: []*compliance.Report{
				{
					Passed: false,
					Error:  ErrResourceCannotUseFallback,
				},
			},
			expectErr: ErrResourceCannotUseFallback,
		},
		{
			name:              "fallback missing",
			resourceCondition: "a >= 3",
			resourceResolved: &eval.Instance{
				Vars: map[string]interface{}{
					"a": 3,
				},
			},
			fallbackCondition: "a == 3",
			expectReports: []*compliance.Report{
				{
					Passed: false,
					Error:  ErrResourceFallbackMissing,
				},
			},
			expectErr: ErrResourceFallbackMissing,
		},
		{
			name:              "iterator not passing",
			resourceCondition: "a > 10",
			resourceResolved:  iterator,
			reportedFields:    []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: false,
					Data: event.Data{
						"a": 6,
					},
				},
				{
					Passed: false,
					Data: event.Data{
						"a": 4,
					},
				},
			},
		},
		{
			name:              "iterator passed",
			resourceCondition: "a > 2",
			resourceResolved:  iterator,
			reportedFields:    []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"a": 14,
					},
				},
			},
		},
		{
			name:              "count equals to zero multiple reports",
			resourceCondition: "count(_) == 0",
			resourceResolved:  iterator,
			reportedFields:    []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: false,
					Data: event.Data{
						"a": 14,
					},
				},
				{
					Passed: false,
					Data: event.Data{
						"a": 6,
					},
				},
				{
					Passed: false,
					Data: event.Data{
						"a": 4,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			resource := compliance.Resource{
				Condition: test.resourceCondition,
			}

			if test.fallbackCondition != "" {
				resource.Fallback = &compliance.Fallback{
					Condition: test.fallbackCondition,
				}
			}

			resolve := func(_ context.Context, _ env.Env, _ string, _ compliance.Resource) (interface{}, error) {
				return test.resourceResolved, nil
			}

			c := &resourceCheck{
				ruleID:         "rule-id",
				resource:       resource,
				resolve:        resolve,
				fallback:       test.fallback,
				reportedFields: test.reportedFields,
			}

			reports := c.check(e)
			assert.Equal(test.expectReports, reports)
			assert.Equal(test.expectErr, reports[0].Error)
		})
	}
}

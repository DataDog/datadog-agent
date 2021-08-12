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

	iterator := &mockIterator{els: []eval.Instance{
		newResolvedInstance(
			eval.NewInstance(
				map[string]interface{}{
					"a": 14,
				}, nil,
			),
			"test-id", "test-resource-type",
		),
		newResolvedInstance(
			eval.NewInstance(
				map[string]interface{}{
					"a": 6,
				}, nil,
			),
			"test-id", "test-resource-type",
		),
		newResolvedInstance(
			eval.NewInstance(
				map[string]interface{}{
					"a": 4,
				}, nil,
			),
			"test-id", "test-resource-type",
		),
	}}
	iterator.On("Next").Return()
	iterator.On("Done").Return()

	tests := []struct {
		name              string
		resourceCondition string
		resourceResolved  resolved
		fallbackCondition string
		fallback          checkable
		reportedFields    []string

		expectReports []*compliance.Report
		expectErr     error
	}{
		{
			name:              "no fallback provided",
			resourceCondition: "a > 3",
			resourceResolved: newResolvedInstance(
				eval.NewInstance(
					map[string]interface{}{
						"a": 4,
						"b": 8,
					}, nil,
				), "test-id", "test-resource-type",
			),
			reportedFields: []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"a": 4,
					},
					Resource: compliance.ReportResource{
						ID:   "test-id",
						Type: "test-resource-type",
					},
				},
			},
		},
		{
			name:              "fallback not used",
			resourceCondition: "a >= 3",
			resourceResolved: newResolvedInstance(
				eval.NewInstance(
					map[string]interface{}{
						"a": 4,
					}, nil,
				), "test-id", "test-resource-type",
			),
			fallbackCondition: "a == 3",
			fallback:          fallback,
			reportedFields:    []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"a": 4,
					},
					Resource: compliance.ReportResource{
						ID:   "test-id",
						Type: "test-resource-type",
					},
				},
			},
		},
		{
			name:              "fallback used",
			resourceCondition: "a >= 3",
			resourceResolved: newResolvedInstance(
				eval.NewInstance(
					map[string]interface{}{
						"a": 3,
					}, nil,
				), "test-id", "test-resource-type",
			),
			fallbackCondition: "a == 3",
			fallback:          fallback,
			expectReports:     fallbackReports,
		},
		{
			name:              "cannot use fallback",
			resourceCondition: "a >= 3",
			resourceResolved: newResolvedIterator(
				newInstanceIterator(
					[]eval.Instance{newResolvedInstance(
						eval.NewInstance(
							map[string]interface{}{
								"a": 3,
							}, nil,
						), "test-id", "test-resource-type",
					)},
				),
			),
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
			resourceResolved: newResolvedInstance(
				eval.NewInstance(
					map[string]interface{}{
						"a": 3,
					}, nil,
				), "test-id", "test-resource-type",
			),
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
			name:              "iterator partially passed",
			resourceCondition: "a > 10",
			resourceResolved:  newResolvedIterator(iterator),
			reportedFields:    []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"a": 14,
					},
					Resource: compliance.ReportResource{
						ID:   "test-id",
						Type: "test-resource-type",
					},
				},
				{
					Passed: false,
					Data: event.Data{
						"a": 6,
					},
					Resource: compliance.ReportResource{
						ID:   "test-id",
						Type: "test-resource-type",
					},
				},
				{
					Passed: false,
					Data: event.Data{
						"a": 4,
					},
					Resource: compliance.ReportResource{
						ID:   "test-id",
						Type: "test-resource-type",
					},
				},
			},
		},
		{
			name:              "count equals to zero multiple reports",
			resourceCondition: "count(_) == 0",
			resourceResolved: &resolvedIterator{
				Iterator: iterator,
			},
			reportedFields: []string{"a"},
			expectReports: []*compliance.Report{
				{
					Passed: false,
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

			resolve := func(_ context.Context, _ env.Env, _ string, _ compliance.BaseResource) (resolved, error) {
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

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

	fallbackReport := &compliance.Report{
		Passed: false,
		Data: event.Data{
			"fallback": true,
		},
	}

	fallback := &mockCheckable{}
	fallback.On("check", e).Return(fallbackReport, nil)

	tests := []struct {
		name              string
		resourceCondition string
		resourceResolved  interface{}
		fallbackCondition string
		fallback          checkable
		reportedFields    []string

		expectReport *compliance.Report
		expectErr    error
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
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"a": 4,
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
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"a": 4,
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
			expectReport:      fallbackReport,
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
			expectErr:         ErrResourceCannotUseFallback,
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
			expectErr:         ErrResourceFallbackMissing,
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

			report, err := c.check(e)
			assert.Equal(test.expectReport, report)
			assert.Equal(test.expectErr, err)
		})

	}
}

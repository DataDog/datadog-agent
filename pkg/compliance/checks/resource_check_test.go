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

func simpleInstanceWithVars(vars eval.VarMap) eval.Instance {
	return eval.NewInstance(vars, nil, eval.RegoInputMap(vars))
}

func TestResourceCheck(t *testing.T) {
	assert := assert.New(t)

	e := &mocks.Env{}
	e.On("MaxEventsPerRun").Return(30)

	iterator := &mockIterator{els: []eval.Instance{
		newResolvedInstance(
			simpleInstanceWithVars(
				eval.VarMap{
					"a": 14,
				},
			),
			"test-id", "test-resource-type",
		),
		newResolvedInstance(
			simpleInstanceWithVars(
				eval.VarMap{
					"a": 6,
				},
			),
			"test-id", "test-resource-type",
		),
		newResolvedInstance(
			simpleInstanceWithVars(
				eval.VarMap{
					"a": 4,
				},
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
		reportedFields    []string

		expectReports []*compliance.Report
		expectErr     error
	}{
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

			resolve := func(_ context.Context, _ env.Env, _ string, _ compliance.ResourceCommon, rego bool) (resolved, error) {
				return test.resourceResolved, nil
			}

			c := &resourceCheck{
				ruleID:         "rule-id",
				resource:       resource,
				resolve:        resolve,
				reportedFields: test.reportedFields,
			}

			reports := c.check(e)
			assert.Equal(test.expectReports, reports)
			assert.Equal(test.expectErr, reports[0].Error)
		})
	}
}

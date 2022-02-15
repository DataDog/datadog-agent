// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/custom"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"
)

func TestNewCustomCheck(t *testing.T) {
	assert := assert.New(t)

	const ruleID = "rule-id"

	expectCheckReport := &compliance.Report{Passed: true}
	expectCheckError := errors.New("check failed")

	customCheckFunc := func(report *compliance.Report, err error) custom.CheckFunc {
		return func(e env.Env, ruleID string, vars map[string]string, expr *eval.IterableExpression) (*compliance.Report, error) {
			return report, err
		}
	}

	tests := []struct {
		name              string
		resource          compliance.Resource
		checkFactory      checkFactoryFunc
		expectError       error
		expectCheckReport *compliance.Report
	}{
		{
			name: "wrong resource kind",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					File: &compliance.File{
						Path: "/etc/bitsy/spider",
					},
				},
			},
			expectError: errors.New("expecting custom resource in custom check"),
		},
		{
			name: "missing check name",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Custom: &compliance.Custom{},
				},
			},
			expectError: errors.New("missing check name in custom check"),
		},
		{
			name: "allowed empty condition",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Custom: &compliance.Custom{
						Name: "check-name",
					},
				},
			},
			checkFactory: func(_ string) custom.CheckFunc {
				return customCheckFunc(expectCheckReport, nil)
			},
			expectCheckReport: expectCheckReport,
		},
		{
			name: "custom check error",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Custom: &compliance.Custom{
						Name: "check-name",
					},
				},
			},
			checkFactory: func(_ string) custom.CheckFunc {
				return customCheckFunc(nil, expectCheckError)
			},
			expectCheckReport: &compliance.Report{
				Passed: false,
				Error:  expectCheckError,
			},
		},
		{
			name: "condition expression failure",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Custom: &compliance.Custom{
						Name: "check-name",
					},
				},
				Condition: "~",
			},
			expectError: errors.New(`1:1: unexpected token "~"`),
		},
		{
			name: "cannot find check by name",
			resource: compliance.Resource{
				ResourceCommon: compliance.ResourceCommon{
					Custom: &compliance.Custom{
						Name: "check-name",
					},
				},
			},
			checkFactory: func(_ string) custom.CheckFunc {
				return nil
			},
			expectError: errors.New("custom check with name: check-name does not exist"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			customCheckFactory = test.checkFactory
			check, err := newCustomCheck(ruleID, test.resource)

			if test.expectError != nil {
				assert.EqualError(err, test.expectError.Error())
				assert.Nil(check)
			} else {
				assert.NotNil(check)
				env := &mocks.Env{}
				reports := check.check(env)
				if test.expectCheckReport.Error != nil {
					assert.EqualError(reports[0].Error, test.expectCheckReport.Error.Error())
				}
				assert.Equal(test.expectCheckReport.Passed, reports[0].Passed)
				assert.Equal(test.expectCheckReport.Data, reports[0].Data)
			}
		})
	}
}

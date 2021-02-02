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

func TestCheckRun(t *testing.T) {
	assert := assert.New(t)

	const (
		ruleID       = "rule-id"
		resourceType = "resource-type"
		resourceID   = "resource-id"
	)

	tests := []struct {
		name        string
		configErr   error
		checkReport *compliance.Report
		checkErr    error
		expectEvent *event.Event
		expectErr   error
	}{
		{
			name: "successful check",
			checkReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					"file.permissions": 0644,
				},
			},
			expectEvent: &event.Event{
				AgentRuleID:  ruleID,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Result:       "passed",
				Data: event.Data{
					"file.permissions": 0644,
				},
			},
		},
		{
			name: "failed check",
			checkReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					"file.permissions": 0644,
				},
			},
			expectEvent: &event.Event{
				AgentRuleID:  ruleID,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Result:       "failed",
				Data: event.Data{
					"file.permissions": 0644,
				},
			},
		},
		{
			name:     "check error",
			checkErr: errors.New("check error"),
			expectEvent: &event.Event{
				AgentRuleID:  ruleID,
				ResourceType: resourceType,
				ResourceID:   resourceID,
				Result:       "error",
				Data: event.Data{
					"error": "check error",
				},
			},
			expectErr: errors.New("check error"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := &mocks.Env{}
			defer env.AssertExpectations(t)

			reporter := &mocks.Reporter{}
			defer reporter.AssertExpectations(t)

			checkable := &mockCheckable{}
			defer checkable.AssertExpectations(t)

			check := &complianceCheck{
				Env: env,

				ruleID:       ruleID,
				resourceType: resourceType,
				resourceID:   resourceID,
				checkable:    checkable,
			}

			if test.configErr == nil {
				env.On("IsLeader").Return(true)
				env.On("Reporter").Return(reporter)
				reporter.On("Report", test.expectEvent).Once()
				checkable.On("check", check).Return(test.checkReport, test.checkErr)
			}

			err := check.Run()
			assert.Equal(test.expectErr, err)
		})
	}
}

func TestCheckRunNoLeader(t *testing.T) {
	const (
		ruleID       = "rule-id"
		resourceType = "resource-type"
		resourceID   = "resource-id"
	)

	assert := assert.New(t)

	env := &mocks.Env{}
	defer env.AssertExpectations(t)

	reporter := &mocks.Reporter{}
	defer reporter.AssertExpectations(t)

	checkable := &mockCheckable{}
	defer checkable.AssertExpectations(t)

	check := &complianceCheck{
		Env: env,

		ruleID:       ruleID,
		resourceType: resourceType,
		resourceID:   resourceID,
		checkable:    checkable,
	}

	// Not leader
	env.On("IsLeader").Return(false)
	checkable.AssertNotCalled(t, "check")

	err := check.Run()
	assert.Nil(err)
}

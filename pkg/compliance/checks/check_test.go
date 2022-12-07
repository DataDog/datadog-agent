// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestCheckRun(t *testing.T) {
	assert := assert.New(t)

	const (
		ruleID       = "rule-id"
		frameworkID  = "cis"
		resourceType = "resource-type"
		resourceID   = "resource-id"
	)

	tests := []struct {
		name         string
		checkReports []*compliance.Report
		expectEvent  *event.Event
		expectErr    error
	}{
		{
			name: "successful check",
			checkReports: []*compliance.Report{
				{
					Passed: true,
					Data: event.Data{
						"file.permissions": 0644,
					},
				},
			},
			expectEvent: &event.Event{
				AgentRuleID:      ruleID,
				AgentFrameworkID: frameworkID,
				AgentVersion:     version.AgentVersion,
				ResourceType:     resourceType,
				ResourceID:       resourceID,
				Result:           "passed",
				Evaluator:        "legacy",
				Data: event.Data{
					"file.permissions": 0644,
				},
			},
		},
		{
			name: "failed check",
			checkReports: []*compliance.Report{
				{
					Passed: false,
					Data: event.Data{
						"file.permissions": 0644,
					},
				},
			},
			expectEvent: &event.Event{
				AgentRuleID:      ruleID,
				AgentFrameworkID: frameworkID,
				AgentVersion:     version.AgentVersion,
				ResourceType:     resourceType,
				ResourceID:       resourceID,
				Result:           "failed",
				Evaluator:        "legacy",
				Data: event.Data{
					"file.permissions": 0644,
				},
			},
		},
		{
			name: "check error",
			checkReports: []*compliance.Report{
				{
					Passed: false,
					Error:  errors.New("check error"),
				},
			},
			expectEvent: &event.Event{
				AgentRuleID:      ruleID,
				AgentFrameworkID: frameworkID,
				AgentVersion:     version.AgentVersion,
				ResourceType:     resourceType,
				ResourceID:       resourceID,
				Result:           "error",
				Evaluator:        "legacy",
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

				ruleID:    ruleID,
				checkable: checkable,
				scope:     resourceType,

				suiteMeta: &compliance.SuiteMeta{Framework: frameworkID},
			}

			env.On("Hostname").Return(resourceID)
			env.On("IsLeader").Return(true)
			env.On("Reporter").Return(reporter)
			reporter.On("Report", mock.MatchedBy(func(e *event.Event) bool {
				e.ExpireAt = time.Time{}
				return cmp.Equal(e, test.expectEvent)
			})).Maybe()
			checkable.On("Check", check).Return(test.checkReports)

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

		ruleID:    ruleID,
		checkable: checkable,
	}

	// Not leader
	env.On("IsLeader").Return(false)
	checkable.AssertNotCalled(t, "check")

	err := check.Run()
	assert.Nil(err)
}

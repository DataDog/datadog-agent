// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/compliance/mocks"

	assert "github.com/stretchr/testify/require"
)

func TestResourceCheck(t *testing.T) {
	assert := assert.New(t)

	e := &mocks.Env{}
	ruleID := "rule-id"
	resource := compliance.Resource{}

	fallbackReport := &compliance.Report{
		Passed: false,
		Data: event.Data{
			"fallback": true,
		},
	}

	fallback := &mockCheckable{}
	fallback.On("check", e).Return(fallbackReport, nil)

	tests := []struct {
		name         string
		checkFn      checkFunc
		fallback     checkable
		expectReport *compliance.Report
		expectErr    error
	}{
		{
			name: "no fallback provided",
			checkFn: func(e env.Env, ruleID string, res compliance.Resource) (*compliance.Report, error) {
				return &compliance.Report{
					Passed: true,
				}, nil
			},
			expectReport: &compliance.Report{
				Passed: true,
			},
		},
		{
			name: "fallback not used",
			checkFn: func(e env.Env, ruleID string, res compliance.Resource) (*compliance.Report, error) {
				return &compliance.Report{
					Passed: false,
				}, nil
			},
			fallback: fallback,
			expectReport: &compliance.Report{
				Passed: false,
			},
		},
		{
			name: "fallback used",
			checkFn: func(e env.Env, ruleID string, res compliance.Resource) (*compliance.Report, error) {
				return nil, ErrResourceUseFallback
			},
			fallback:     fallback,
			expectReport: fallbackReport,
		},
		{
			name: "fallback missing",
			checkFn: func(e env.Env, ruleID string, res compliance.Resource) (*compliance.Report, error) {
				return nil, ErrResourceUseFallback
			},
			expectErr: ErrResourceFallbackMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := &resourceCheck{
				ruleID:   ruleID,
				resource: resource,
				checkFn:  test.checkFn,
				fallback: test.fallback,
			}

			report, err := c.check(e)
			assert.Equal(test.expectReport, report)
			assert.Equal(test.expectErr, err)
		})

	}
}

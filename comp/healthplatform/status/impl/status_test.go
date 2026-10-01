// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package statusimpl

import (
	"bytes"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	egressmock "github.com/DataDog/datadog-agent/comp/healthplatform/egress/mock"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

func newTestProvider(t *testing.T, cfgOverrides map[string]interface{}, opts ...storemock.Option) statusProvider {
	provides := NewComponent(Requires{
		Config: config.NewMockWithOverrides(t, cfgOverrides),
		Store:  storemock.New(t, opts...),
		Egress: egressmock.New(),
	})
	return provides.StatusProvider.Provider.(statusProvider)
}

func TestStatusOutputEnabled(t *testing.T) {
	sp := newTestProvider(t, map[string]interface{}{"health_platform.enabled": true},
		storemock.WithIssue(&healthplatformpayload.Issue{Id: "1", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_HIGH}),
		storemock.WithIssue(&healthplatformpayload.Issue{Id: "2", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM}),
		storemock.WithIssue(&healthplatformpayload.Issue{Id: "3", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_LOW}),
	)

	tests := []struct {
		name       string
		assertFunc func(t *testing.T)
	}{
		{"JSON", func(t *testing.T) {
			stats := make(map[string]interface{})
			err := sp.JSON(false, stats)
			assert.NoError(t, err)

			hp, ok := stats["healthPlatform"].(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, true, hp["enabled"])
			assert.Equal(t, 3, hp["activeIssues"])
			assert.Equal(t, 1, hp["highSeverityIssues"])
			assert.Equal(t, 1, hp["mediumSeverityIssues"])
			assert.Equal(t, 1, hp["lowSeverityIssues"])
			assert.Equal(t, 0, hp["unknownSeverityIssues"])
			assert.Equal(t, true, hp["egressHealthy"])
		}},
		{"Text", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := sp.Text(false, b)
			assert.NoError(t, err)
			assert.NotEmpty(t, b.String())
		}},
		{"HTML", func(t *testing.T) {
			b := new(bytes.Buffer)
			err := sp.HTML(false, b)
			assert.NoError(t, err)
			assert.NotEmpty(t, b.String())
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(t)
		})
	}
}

func TestStatusOutputDisabled(t *testing.T) {
	sp := newTestProvider(t, map[string]interface{}{"health_platform.enabled": false})

	stats := make(map[string]interface{})
	err := sp.JSON(false, stats)
	assert.NoError(t, err)

	hp, ok := stats["healthPlatform"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, false, hp["enabled"])
	assert.Nil(t, hp["activeIssues"])

	b := new(bytes.Buffer)
	assert.NoError(t, sp.Text(false, b))
	assert.NotEmpty(t, b.String())
}

// TestStatusOutputUnspecifiedSeverity verifies an issue with unspecified
// severity is counted separately rather than folded into "low", which would
// understate its severity.
func TestStatusOutputUnspecifiedSeverity(t *testing.T) {
	sp := newTestProvider(t, map[string]interface{}{"health_platform.enabled": true},
		storemock.WithIssue(&healthplatformpayload.Issue{Id: "1", Severity: healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_UNSPECIFIED}),
	)

	stats := make(map[string]interface{})
	err := sp.JSON(false, stats)
	assert.NoError(t, err)

	hp, ok := stats["healthPlatform"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, 1, hp["activeIssues"])
	assert.Equal(t, 0, hp["lowSeverityIssues"])
	assert.Equal(t, 1, hp["unknownSeverityIssues"])
}

func TestNameAndSection(t *testing.T) {
	sp := newTestProvider(t, map[string]interface{}{"health_platform.enabled": true})
	assert.Equal(t, "Health Platform", sp.Name())
	assert.Equal(t, "Health Platform", sp.Section())
}

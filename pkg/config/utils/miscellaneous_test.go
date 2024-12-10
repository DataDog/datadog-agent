// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"

	"github.com/stretchr/testify/assert"
)

func TestIsCoreAgentEnabled(t *testing.T) {

	tests := []struct {
		name      string
		expected  bool
		setConfig func(m model.Config)
	}{
		{
			name:     "core_agent.enabled false",
			expected: false,
			setConfig: func(m model.Config) {
				m.SetWithoutSource("core_agent.enabled", false)
			},
		},
		{
			name:     "All enable_payloads.enabled false",
			expected: false,
			setConfig: func(m model.Config) {
				m.SetWithoutSource("enable_payloads.events", false)
				m.SetWithoutSource("enable_payloads.series", false)
				m.SetWithoutSource("enable_payloads.service_checks", false)
				m.SetWithoutSource("enable_payloads.sketches", false)
			},
		},
		{
			name:     "Some enable_payloads.enabled false",
			expected: true,
			setConfig: func(m model.Config) {
				m.SetWithoutSource("enable_payloads.events", false)
				m.SetWithoutSource("enable_payloads.series", true)
				m.SetWithoutSource("enable_payloads.service_checks", false)
				m.SetWithoutSource("enable_payloads.sketches", true)
			},
		},
		{
			name:      "default values",
			expected:  true,
			setConfig: func(_ model.Config) {},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			test.setConfig(mockConfig)
			assert.Equal(t,
				test.expected, IsCoreAgentEnabled(mockConfig),
				"Was expecting IsCoreAgentEnabled to return", test.expected)
		})
	}
}

func TestIsAPMEnabled(t *testing.T) {
	tests := []struct {
		name                                      string
		apmEnabled, errorTrackingEnable, expected bool
	}{
		{
			name:                "APM enabled and Error Tracking standalone disabled",
			apmEnabled:          false,
			errorTrackingEnable: false,
			expected:            false,
		},
		{
			name:                "APM enabled and Error Tracking standalone disabled",
			apmEnabled:          true,
			errorTrackingEnable: false,
			expected:            true,
		},
		{
			name:                "APM disabled and Error Tracking standalone enabled",
			apmEnabled:          false,
			errorTrackingEnable: true,
			expected:            true,
		},
		{
			name:                "APM enabled and Error Tracking standalone enabled",
			apmEnabled:          true,
			errorTrackingEnable: true,
			expected:            true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("apm_config.enabled", test.apmEnabled)
			mockConfig.SetWithoutSource("apm_config.error_tracking_standalone.enabled", test.errorTrackingEnable)
			assert.Equal(t,
				test.expected, IsAPMEnabled(mockConfig),
				"Was expecting IsAPMEnabled to return", test.expected)
		})
	}
}

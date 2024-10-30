// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestIsDefaultPayloadsEnabled(t *testing.T) {

	tests := []struct {
		name      string
		expected  bool
		setConfig func(m model.Config)
	}{
		{
			name:     "default_payloads.enabled false",
			expected: false,
			setConfig: func(m model.Config) {
				m.SetWithoutSource("default_payloads.enabled", false)
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
				test.expected, IsDefaultPayloadsEnabled(mockConfig),
				"Was expecting IsDefaultPayloadsEnabled to return", test.expected)
		})
	}
}

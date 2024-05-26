// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package networkpath

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestNewCheckConfig(t *testing.T) {
	coreconfig.Datadog.SetDefault("network_devices.namespace", "my-namespace")
	tests := []struct {
		name           string
		rawInstance    integration.Data
		rawInitConfig  integration.Data
		expectedConfig *CheckConfig
		expectedError  string
	}{
		{
			name: "valid minimal config",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
			},
		},
		{
			name:          "invalid raw instance json",
			rawInstance:   []byte(`{{{`),
			expectedError: "invalid instance config",
		},
		{
			name:          "invalid raw instance json",
			rawInstance:   []byte(`hostname: 1.2.3.4`),
			rawInitConfig: []byte(`{{{`),
			expectedError: "invalid init_config",
		},
		{
			name: "invalid min_collection_interval",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: -1
`),
			expectedError: "min collection interval must be > 0",
		},
		{
			name: "min_collection_interval from instance",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
			},
		},
		{
			name: "min_collection_interval from init_config",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(10) * time.Second,
				Namespace:             "my-namespace",
			},
		},
		{
			name: "min_collection_interval default value",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(1) * time.Minute,
				Namespace:             "my-namespace",
			},
		},
		{
			name: "source and destination service config",
			rawInstance: []byte(`
hostname: 1.2.3.4
source_service: service-a
destination_service: service-b
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				SourceService:         "service-a",
				DestinationService:    "service-b",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewCheckConfig(tt.rawInstance, tt.rawInitConfig)
			assert.Equal(t, tt.expectedConfig, config)
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			}
		})
	}
}

func TestFirstNonZero(t *testing.T) {
	tests := []struct {
		name          string
		values        []time.Duration
		expectedValue time.Duration
	}{
		{
			name:          "no value",
			expectedValue: 0,
		},
		{
			name: "one value",
			values: []time.Duration{
				time.Duration(10) * time.Second,
			},
			expectedValue: time.Duration(10) * time.Second,
		},
		{
			name: "multiple values - select first",
			values: []time.Duration{
				time.Duration(10) * time.Second,
				time.Duration(20) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(10) * time.Second,
		},
		{
			name: "multiple values - select second",
			values: []time.Duration{
				time.Duration(0) * time.Second,
				time.Duration(20) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(20) * time.Second,
		},
		{
			name: "multiple values - select third",
			values: []time.Duration{
				time.Duration(0) * time.Second,
				time.Duration(0) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(30) * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedValue, firstNonZero(tt.values...))
		})
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name          string
		configFactory func(*testing.T) config.Component
		expectedState Config
	}{
		{
			name: "default_config",
			configFactory: func(t *testing.T) config.Component {
				mockConfig := config.NewMock(t)
				mockConfig.SetWithoutSource("site", "datadoghq.com")
				return mockConfig
			},
			expectedState: Config{
				Site:           "datadoghq.com",
				DDRegistries:   map[string]struct{}{"gcr.io/datadoghq": {}, "docker.io/datadog": {}, "public.ecr.aws/datadog": {}},
				RCClient:       nil,
				MaxInitRetries: 5,
				InitRetryDelay: 1 * time.Second,
			},
		},
		{
			name: "custom_dd_registries",
			configFactory: func(t *testing.T) config.Component {
				mockConfig := config.NewMock(t)
				mockConfig.SetWithoutSource("site", "datadoghq.com")
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.default_dd_registries", []string{"helloworld.io/datadog"})
				return mockConfig
			},
			expectedState: Config{
				Site:           "datadoghq.com",
				DDRegistries:   map[string]struct{}{"helloworld.io/datadog": {}},
				RCClient:       nil,
				MaxInitRetries: 5,
				InitRetryDelay: 1 * time.Second,
			},
		},
		{
			name: "configured_site",
			configFactory: func(t *testing.T) config.Component {
				mockConfig := config.NewMock(t)
				mockConfig.SetWithoutSource("site", "datad0g.com")
				return mockConfig
			},
			expectedState: Config{
				Site:           "datad0g.com",
				DDRegistries:   map[string]struct{}{"gcr.io/datadoghq": {}, "docker.io/datadog": {}, "public.ecr.aws/datadog": {}},
				RCClient:       nil,
				MaxInitRetries: 5,
				InitRetryDelay: 1 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := tt.configFactory(t)
			result := NewConfig(mockConfig, nil)

			require.Equal(t, tt.expectedState, result)
		})
	}
}

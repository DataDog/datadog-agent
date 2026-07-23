// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package config

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func TestValidateSidecarConfig_Valid(t *testing.T) {
	tests := []struct {
		name   string
		config Sidecar
	}{
		{
			name: "valid config with all fields",
			config: Sidecar{
				Image:                "datadog/appsec-processor:latest",
				ImageTag:             "v1.0",
				Port:                 8080,
				HealthPort:           8081,
				RunAsUser:            65532,
				CPURequest:           "100m",
				CPULimit:             "200m",
				MemoryRequest:        "128Mi",
				MemoryLimit:          "256Mi",
				BodyParsingSizeLimit: "10000000",
			},
		},
		{
			name: "valid config with minimal fields",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       8080,
				HealthPort: 9090,
				RunAsUser:  65532,
			},
		},
		{
			name: "valid config with port 1 and 65535",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       1,
				HealthPort: 65535,
				RunAsUser:  65532,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSidecarConfig(tt.config)
			assert.NoError(t, err)
		})
	}
}

func TestValidateSidecarConfig_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		config      Sidecar
		expectedErr string
	}{
		{
			name: "missing image",
			config: Sidecar{
				Port:       8080,
				HealthPort: 8081,
			},
			expectedErr: "sidecar image is required",
		},
		{
			name: "invalid port - zero",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       0,
				HealthPort: 8081,
			},
			expectedErr: "sidecar.port must be between 1 and 65535",
		},
		{
			name: "invalid port - negative",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       -1,
				HealthPort: 8081,
			},
			expectedErr: "sidecar.port must be between 1 and 65535",
		},
		{
			name: "invalid port - too high",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       65536,
				HealthPort: 8081,
			},
			expectedErr: "sidecar.port must be between 1 and 65535",
		},
		{
			name: "invalid health port - zero",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       8080,
				HealthPort: 0,
			},
			expectedErr: "sidecar.health_port must be between 1 and 65535",
		},
		{
			name: "invalid health port - negative",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       8080,
				HealthPort: -1,
			},
			expectedErr: "sidecar.health_port must be between 1 and 65535",
		},
		{
			name: "invalid health port - too high",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       8080,
				HealthPort: 65536,
			},
			expectedErr: "sidecar.health_port must be between 1 and 65535",
		},
		{
			name: "port and health port are the same",
			config: Sidecar{
				Image:      "datadog/appsec-processor:latest",
				Port:       8080,
				HealthPort: 8080,
			},
			expectedErr: "sidecar.port and sidecar.health_port cannot be the same",
		},
		{
			name: "multiple validation errors",
			config: Sidecar{
				// Missing image
				Port:       0,     // Invalid port
				HealthPort: 70000, // Invalid health port
			},
			expectedErr: "sidecar image is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSidecarConfig(tt.config)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestFromComponent_SidecarMode_ValidConfig(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode":                            "sidecar",
		"admission_controller.appsec.sidecar.image":                     "datadog/appsec:custom",
		"admission_controller.appsec.sidecar.port":                      8080,
		"admission_controller.appsec.sidecar.health_port":               8081,
		"admission_controller.appsec.sidecar.resources.requests.cpu":    "100m",
		"admission_controller.appsec.sidecar.resources.requests.memory": "128Mi",
		"admission_controller.appsec.sidecar.resources.limits.cpu":      "200m",
		"admission_controller.appsec.sidecar.resources.limits.memory":   "256Mi",
		"admission_controller.appsec.sidecar.body_parsing_size_limit":   "10000000",
		"appsec.proxy.enabled":                                          true,
		"appsec.proxy.proxies":                                          []string{"istio"},
		"cluster_agent.appsec.injector.enabled":                         true,
	})

	mockLogger := logmock.New(t)
	config := FromComponent(mockConfig, mockLogger)

	assert.Equal(t, InjectionModeSidecar, config.Mode)
	assert.Equal(t, "datadog/appsec:custom", config.Sidecar.Image)
	assert.Equal(t, 8080, config.Sidecar.Port)
	assert.Equal(t, 8081, config.Sidecar.HealthPort)
	assert.Equal(t, "100m", config.Sidecar.CPURequest)
	assert.Equal(t, "128Mi", config.Sidecar.MemoryRequest)
	assert.Equal(t, "200m", config.Sidecar.CPULimit)
	assert.Equal(t, "256Mi", config.Sidecar.MemoryLimit)
	assert.Equal(t, "10000000", config.Sidecar.BodyParsingSizeLimit)
}

func TestFromComponent_SidecarMode_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]any
	}{
		{
			name: "missing image",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode":              "sidecar",
				"admission_controller.appsec.sidecar.port":        8080,
				"admission_controller.appsec.sidecar.health_port": 8081,
				"appsec.proxy.enabled":                            true,
				"appsec.proxy.proxies":                            []string{"istio"},
				"cluster_agent.appsec.injector.enabled":           true,
			},
		},
		{
			name: "invalid port",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode":                "sidecar",
				"admission_controller.appsec.sidecar.image":         "datadog/appsec:test",
				"cluster_agent.appsec.injector.sidecar.port":        0,
				"cluster_agent.appsec.injector.sidecar.health_port": 8081,
				"appsec.proxy.enabled":                              true,
				"appsec.proxy.proxies":                              []string{"istio"},
				"cluster_agent.appsec.injector.enabled":             true,
			},
		},
		{
			name: "same port and health port",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode":                "sidecar",
				"admission_controller.appsec.sidecar.image":         "datadog/appsec:test",
				"cluster_agent.appsec.injector.sidecar.port":        8080,
				"cluster_agent.appsec.injector.sidecar.health_port": 8080,
				"appsec.proxy.enabled":                              true,
				"appsec.proxy.proxies":                              []string{"istio"},
				"cluster_agent.appsec.injector.enabled":             true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, tt.config)
			mockLogger := logmock.New(t)

			// Should not panic, but validation errors should be logged
			config := FromComponent(mockConfig, mockLogger)
			assert.NotNil(t, config)
		})
	}
}

func TestFromComponent_ExternalMode_ValidConfig(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode":                        "external",
		"cluster_agent.appsec.injector.processor.service.name":      "appsec-processor",
		"cluster_agent.appsec.injector.processor.service.namespace": "datadog",
		"appsec.proxy.processor.port":                               8080,
		"appsec.proxy.enabled":                                      true,
		"appsec.proxy.proxies":                                      []string{"istio"},
		"cluster_agent.appsec.injector.enabled":                     true,
	})

	mockLogger := logmock.New(t)
	config := FromComponent(mockConfig, mockLogger)

	assert.Equal(t, InjectionModeExternal, config.Mode)
	assert.Equal(t, "appsec-processor", config.Processor.ServiceName)
	assert.Equal(t, "datadog", config.Processor.Namespace)
	assert.Equal(t, 8080, config.Processor.Port)
}

func TestFromComponent_ExternalMode_MissingServiceName(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode": "external",
		// Missing service name
		"appsec.proxy.processor.port":           8080,
		"appsec.proxy.enabled":                  true,
		"appsec.proxy.proxies":                  []string{"istio"},
		"cluster_agent.appsec.injector.enabled": true,
	})

	mockLogger := logmock.New(t)

	// Should not panic, but validation errors should be logged
	config := FromComponent(mockConfig, mockLogger)
	assert.NotNil(t, config)
	assert.Equal(t, InjectionModeExternal, config.Mode)
}

func TestFromComponent_DefaultsToSidecar(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode":                "invalid-mode",
		"cluster_agent.appsec.injector.sidecar.image":       "datadog/appsec:latest",
		"cluster_agent.appsec.injector.sidecar.port":        8080,
		"cluster_agent.appsec.injector.sidecar.health_port": 8081,
		"appsec.proxy.enabled":                              true,
		"appsec.proxy.proxies":                              []string{"istio"},
		"cluster_agent.appsec.injector.enabled":             true,
	})

	mockLogger := logmock.New(t)
	config := FromComponent(mockConfig, mockLogger)

	// Should default to sidecar mode
	assert.Equal(t, InjectionModeSidecar, config.Mode)
}

func TestFromComponent_SidecarUDSDefaults(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode":              "sidecar",
		"admission_controller.appsec.sidecar.image":       "datadog/appsec:latest",
		"admission_controller.appsec.sidecar.port":        8080,
		"admission_controller.appsec.sidecar.health_port": 8081,
		"appsec.proxy.enabled":                            true,
		"appsec.proxy.proxies":                            []string{"envoy-gateway"},
		"cluster_agent.appsec.injector.enabled":           true,
	})

	mockLogger := logmock.New(t)
	cfg := FromComponent(mockConfig, mockLogger)

	assert.Equal(t, "/var/run/datadog/extproc.sock", cfg.Sidecar.UDSPath)
	assert.Equal(t, int64(65532), cfg.Sidecar.RunAsUser)
}

func TestValidateSidecarConfig_UDSPath(t *testing.T) {
	base := Sidecar{
		Image:      "datadog/appsec:latest",
		Port:       8080,
		HealthPort: 8081,
		RunAsUser:  65532,
	}

	t.Run("relative path is rejected", func(t *testing.T) {
		cfg := base
		cfg.UDSPath = "relative/path"
		err := validateSidecarConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sidecar.uds_path must be an absolute path")
	})

	t.Run("path over 100 chars is rejected", func(t *testing.T) {
		cfg := base
		cfg.UDSPath = "/" + strings.Repeat("a", 119)
		err := validateSidecarConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sidecar.uds_path must be at most 100 characters")
	})

	t.Run("root directory path is rejected", func(t *testing.T) {
		cfg := base
		cfg.UDSPath = "/sock.sock"
		err := validateSidecarConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sidecar.uds_path must be inside a non-root directory")
	})

	t.Run("zero run_as_user is rejected", func(t *testing.T) {
		cfg := base
		cfg.RunAsUser = 0
		err := validateSidecarConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sidecar.run_as_user must be greater than 0")
	})

	t.Run("negative run_as_user is rejected", func(t *testing.T) {
		cfg := base
		cfg.RunAsUser = -1
		err := validateSidecarConfig(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sidecar.run_as_user must be greater than 0")
	})

	t.Run("valid uds config passes", func(t *testing.T) {
		cfg := base
		cfg.UDSPath = "/var/run/datadog/extproc.sock"
		cfg.RunAsUser = 65532
		err := validateSidecarConfig(cfg)
		assert.NoError(t, err)
	})
}

func TestAllProxyTypes_ContainsGKEGateway(t *testing.T) {
	assert.True(t, slices.Contains(AllProxyTypes, ProxyTypeGKEGateway),
		"AllProxyTypes should contain ProxyTypeGKEGateway")
}

func TestFromComponent_GKEGatewayClasses(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode":    "external",
		"appsec.proxy.gke.gateway_classes":      []string{"foo"},
		"appsec.proxy.enabled":                  true,
		"cluster_agent.appsec.injector.enabled": true,
	})

	mockLogger := logmock.New(t)
	cfg := FromComponent(mockConfig, mockLogger)

	assert.Equal(t, []string{"foo"}, cfg.Product.GKE.GatewayClasses)
}

func TestFromComponent_ProxiesUnknownTypeDropped(t *testing.T) {
	tests := []struct {
		name            string
		proxies         []string
		wantContains    []ProxyType
		wantNotContains []ProxyType
	}{
		{
			name:            "known gke-gateway kept, bogus-proxy dropped",
			proxies:         []string{"gke-gateway", "bogus-proxy"},
			wantContains:    []ProxyType{ProxyTypeGKEGateway},
			wantNotContains: []ProxyType{ProxyType("bogus-proxy")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, map[string]any{
				"cluster_agent.appsec.injector.mode":    "external",
				"appsec.proxy.proxies":                  tt.proxies,
				"appsec.proxy.enabled":                  true,
				"cluster_agent.appsec.injector.enabled": true,
			})

			mockLogger := logmock.New(t)
			cfg := FromComponent(mockConfig, mockLogger)

			for _, pt := range tt.wantContains {
				_, ok := cfg.Product.Proxies[pt]
				assert.True(t, ok, "Proxies should contain %q", pt)
			}
			for _, pt := range tt.wantNotContains {
				_, ok := cfg.Product.Proxies[pt]
				assert.False(t, ok, "Proxies should NOT contain %q", pt)
			}
		})
	}
}

func TestProcessorString(t *testing.T) {
	tests := []struct {
		name      string
		processor Processor
		expected  string
	}{
		{
			name: "with address",
			processor: Processor{
				Address: "appsec-processor.datadog.svc",
				Port:    8080,
			},
			expected: "appsec-processor.datadog.svc:8080",
		},
		{
			name: "without address - derived from service name and namespace",
			processor: Processor{
				ServiceName: "appsec-processor",
				Namespace:   "datadog",
				Port:        8080,
			},
			expected: "appsec-processor.datadog.svc:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.processor.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

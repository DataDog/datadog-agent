// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRCClient is a lightweight mock that implements RemoteConfigClient
type mockRCClient struct {
	configs     map[string]state.RawConfig
	subscribers map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))
}

// loadTestConfigFile loads a test data file and converts it to the format returned by rcClient.GetConfigs()
func loadTestConfigFile(filename string) (map[string]state.RawConfig, error) {
	// Read the test data file
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file %s: %v", filename, err)
	}

	// Parse as map of config names to repository configs
	var repoConfigs map[string]RepositoryConfig
	if err := json.Unmarshal(data, &repoConfigs); err != nil {
		return nil, fmt.Errorf("failed to parse test data: %v", err)
	}

	// Convert each repository config to RawConfig format (as rcClient.GetConfigs() would return)
	rawConfigs := make(map[string]state.RawConfig)
	for configName, repoConfig := range repoConfigs {
		configJSON, err := json.Marshal(repoConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config %s: %v", configName, err)
		}
		rawConfigs[configName] = state.RawConfig{Config: configJSON}
	}

	return rawConfigs, nil
}

func newMockRCClient(filename string) *mockRCClient {
	rawConfigs, err := loadTestConfigFile(filename)
	if err != nil {
		panic(err)
	}

	return &mockRCClient{
		configs:     rawConfigs,
		subscribers: make(map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))),
	}
}

func (m *mockRCClient) Subscribe(_ string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	log.Debugf("Would subscribe called with product on RCClient: %s", product)
}

func (m *mockRCClient) GetConfigs(product string) map[string]state.RawConfig {
	return m.configs
}

func TestNewImageResolver(t *testing.T) {
	t.Run("with_remote_config_client", func(t *testing.T) {
		mockClient := newMockRCClient("image_resolver_multi_repo.json")
		resolver := NewImageResolver(mockClient)

		_, ok := resolver.(*remoteConfigImageResolver)
		assert.True(t, ok, "Should return remoteConfigImageResolver when rcClient is not nil")
	})

	t.Run("without_remote_config_client", func(t *testing.T) {
		resolver := NewImageResolver(nil)

		_, ok := resolver.(*noOpImageResolver)
		assert.True(t, ok, "Should return noOpImageResolver when rcClient is nil")
	})
}

func TestNoOpImageResolver(t *testing.T) {
	resolver := newNoOpImageResolver()

	testCases := []struct {
		name       string
		registry   string
		repository string
		tag        string
	}{
		{
			name:       "full_image_reference",
			registry:   "gcr.io/datadoghq",
			repository: "dd-lib-python-init",
			tag:        "latest",
		},
		{
			name:       "versioned_tag",
			registry:   "gcr.io/datadoghq",
			repository: "dd-lib-java-init",
			tag:        "v1.2.3",
		},
		{
			name:       "simple_registry",
			registry:   "docker.io",
			repository: "my-app",
			tag:        "latest",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := resolver.Resolve(tc.registry, tc.repository, tc.tag)
			assert.Nil(t, resolved, "NoOpImageResolver should never return a resolved image")
			assert.False(t, ok, "NoOpImageResolver should always return false")
		})
	}
}

func TestRemoteConfigImageResolver_processUpdate(t *testing.T) {
	// Create a resolver directly to test private functions
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	// Load test data to use in tests
	testConfigs, err := loadTestConfigFile("image_resolver_multi_repo.json")
	require.NoError(t, err)

	t.Run("multiple_repositories", func(t *testing.T) {
		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		// Test the private processUpdate method directly
		resolver.processUpdate(testConfigs, applyStateCallback)

		// Verify the cache was populated
		assert.Len(t, resolver.imageMappings, 3) // python, java, js
		assert.Contains(t, resolver.imageMappings, "dd-lib-python-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-java-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-js-init")

		// Verify apply statuses were called
		assert.Len(t, appliedStatuses, 3)
		for _, status := range appliedStatuses {
			assert.Equal(t, state.ApplyStateAcknowledged, status.State)
		}
	})

	t.Run("empty_update_clears_cache", func(t *testing.T) {
		update := map[string]state.RawConfig{}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		assert.Len(t, resolver.imageMappings, 0)
		assert.Len(t, appliedStatuses, 0)
	})
}

// TestImageResolverEmptyConfig tests the behavior with no remote config data
func TestImageResolverEmptyConfig(t *testing.T) {
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	resolver.processUpdate(map[string]state.RawConfig{}, func(string, state.ApplyStatus) {})

	resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
	assert.False(t, ok, "Resolution should fail with empty config")
	assert.Nil(t, resolved, "Should not return resolved image with empty cache")
}

func TestRemoteConfigImageResolver_Resolve(t *testing.T) {
	mockRCClient := newMockRCClient("image_resolver_multi_repo.json")
	resolver := newRemoteConfigImageResolver(mockRCClient)

	testCases := []struct {
		name           string
		registry       string
		repository     string
		tag            string
		expectedResult string
		expectedOK     bool
	}{
		{
			name:           "successful_resolution_latest",
			registry:       "gcr.io/datadoghq",
			repository:     "dd-lib-python-init",
			tag:            "latest",
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123",
			expectedOK:     true,
		},
		{
			name:           "successful_resolution_versioned",
			registry:       "gcr.io/datadoghq",
			repository:     "dd-lib-python-init",
			tag:            "v3",
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:def456",
			expectedOK:     true,
		},
		{
			name:       "non_existent_repository",
			registry:   "gcr.io/datadoghq",
			repository: "dd-lib-hello-init",
			tag:        "latest",
			expectedOK: false,
		},
		{
			name:       "non_existent_tag",
			registry:   "gcr.io/datadoghq",
			repository: "dd-lib-python-init",
			tag:        "v2",
			expectedOK: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := resolver.Resolve(tc.registry, tc.repository, tc.tag)
			assert.Equal(t, tc.expectedOK, ok)

			if tc.expectedOK {
				require.NotNil(t, resolved, "Should have resolved image when expectedOK is true")
				assert.Equal(t, tc.expectedResult, resolved.FullImageRef, "Resolved image should match expected")
			} else {
				assert.Nil(t, resolved, "Should not have resolved image when expectedOK is false")
			}
		})
	}

	// Test empty cache
	t.Run("empty_cache", func(t *testing.T) {
		emptyResolver := &remoteConfigImageResolver{
			imageMappings: make(map[string]map[string]ResolvedImage),
		}
		resolved, ok := emptyResolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Resolution should fail with empty cache")
		assert.Nil(t, resolved, "Should not return resolved image with empty cache")
	})
}

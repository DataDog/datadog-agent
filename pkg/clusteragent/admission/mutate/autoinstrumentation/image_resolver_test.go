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

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewImageResolver(t *testing.T) {
	t.Run("with_remote_config_client", func(t *testing.T) {
		mockClient := (*rcclient.Client)(nil)
		resolver := NewImageResolver(mockClient)

		_, ok := resolver.(*noOpImageResolver)
		assert.True(t, ok, "Should return noOpImageResolver when rcClient is nil")
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

// loadRepositoryConfigs loads repository configurations from a JSON file.
// Supports both single config objects and arrays of configs, always returns a slice.
func loadRepositoryConfigs(t *testing.T, filename string) []RepositoryConfig {
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	require.NoError(t, err, "Failed to read test data file %s", filename)

	var configs []RepositoryConfig
	if err = json.Unmarshal(data, &configs); err == nil {
		return configs
	}

	// If that fails, try as single config
	var config RepositoryConfig
	err = json.Unmarshal(data, &config)
	require.NoError(t, err, "Failed to unmarshal repository config(s) from %s", filename)

	return []RepositoryConfig{config}
}

func TestRemoteConfigImageResolver_processUpdate(t *testing.T) {
	// Create resolver without remote config client for testing
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	allConfigs := loadRepositoryConfigs(t, "image_resolver_multi_repo.json")
	require.Len(t, allConfigs, 3) // python, java, js

	var pythonConfig, javaConfig RepositoryConfig
	for _, config := range allConfigs {
		switch config.RepositoryName {
		case "dd-lib-python-init":
			pythonConfig = config
		case "dd-lib-java-init":
			javaConfig = config
		}
	}

	pythonJSON, err := json.Marshal(pythonConfig)
	require.NoError(t, err)
	javaJSON, err := json.Marshal(javaConfig)
	require.NoError(t, err)

	t.Run("multiple_repositories", func(t *testing.T) {
		update := map[string]state.RawConfig{
			"config1": {Config: pythonJSON},
			"config2": {Config: javaJSON},
		}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Verify both repositories are in the cache
		assert.Len(t, resolver.imageMappings, 2)
		assert.Contains(t, resolver.imageMappings, "dd-lib-python-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-java-init")

		// Verify python repo content (from testdata/image_resolver_multi_repo.json)
		pythonRepo := resolver.imageMappings["dd-lib-python-init"]
		assert.Len(t, pythonRepo, 2) // latest, v3
		assert.Contains(t, pythonRepo, "latest")
		assert.Contains(t, pythonRepo, "v3")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123", pythonRepo["latest"].FullImageRef)
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-python-init@sha256:def456", pythonRepo["v3"].FullImageRef)

		// Verify java repo content (from testdata/image_resolver_multi_repo.json)
		javaRepo := resolver.imageMappings["dd-lib-java-init"]
		assert.Len(t, javaRepo, 2) // latest, v1
		assert.Contains(t, javaRepo, "latest")
		assert.Contains(t, javaRepo, "v1")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-java-init@sha256:ghi789", javaRepo["latest"].FullImageRef)

		// Verify apply statuses
		assert.Len(t, appliedStatuses, 2)
		assert.Equal(t, state.ApplyStateAcknowledged, appliedStatuses["config1"].State)
		assert.Equal(t, state.ApplyStateAcknowledged, appliedStatuses["config2"].State)
	})

	t.Run("empty_update_clears_cache", func(t *testing.T) {
		update := map[string]state.RawConfig{}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Cache should be empty
		assert.Len(t, resolver.imageMappings, 0)
		assert.Len(t, appliedStatuses, 0)
	})

	t.Run("multi_repository_from_testdata", func(t *testing.T) {
		// Load multi-repository configuration from testdata
		configs := loadRepositoryConfigs(t, "image_resolver_multi_repo.json")
		require.Len(t, configs, 3) // python, java, js

		// Convert to update format
		update := make(map[string]state.RawConfig)
		for i, config := range configs {
			configJSON, err := json.Marshal(config)
			require.NoError(t, err)
			update[fmt.Sprintf("config%d", i)] = state.RawConfig{Config: configJSON}
		}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Verify all three repositories are loaded
		assert.Len(t, resolver.imageMappings, 3)
		assert.Contains(t, resolver.imageMappings, "dd-lib-python-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-java-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-js-init")

		// Verify JavaScript repo (new in multi-repo testdata)
		jsRepo := resolver.imageMappings["dd-lib-js-init"]
		assert.Len(t, jsRepo, 2) // latest, v5
		assert.Contains(t, jsRepo, "latest")
		assert.Contains(t, jsRepo, "v5")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-js-init@sha256:js123", jsRepo["latest"].FullImageRef)

		// Verify apply statuses for all configs
		assert.Len(t, appliedStatuses, 3)
		for i := 0; i < 3; i++ {
			configKey := fmt.Sprintf("config%d", i)
			assert.Equal(t, state.ApplyStateAcknowledged, appliedStatuses[configKey].State)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		update := map[string]state.RawConfig{
			"invalid_config": {Config: []byte("invalid json")},
		}

		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(update, applyStateCallback)

		// Cache should be empty
		assert.Len(t, resolver.imageMappings, 0)

		// Error status should be recorded
		assert.Len(t, appliedStatuses, 1)
		assert.Equal(t, state.ApplyStateError, appliedStatuses["invalid_config"].State)
		assert.Contains(t, appliedStatuses["invalid_config"].Error, "invalid character")
	})
}

// TestImageResolverWithTestData tests the image resolver using testdata files
// This follows the same pattern as target_mutator_test.go
func TestImageResolverWithTestData(t *testing.T) {
	testCases := map[string]struct {
		registry         string
		repository       string
		tag              string
		expectedImage    string
		expectedResolved bool
	}{
		"python_latest": {
			registry:         "gcr.io/datadoghq",
			repository:       "dd-lib-python-init",
			tag:              "latest",
			expectedImage:    "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123",
			expectedResolved: true,
		},
		"python_v3": {
			registry:         "gcr.io/datadoghq",
			repository:       "dd-lib-python-init",
			tag:              "v3",
			expectedImage:    "gcr.io/datadoghq/dd-lib-python-init@sha256:def456",
			expectedResolved: true,
		},
		"java_latest": {
			registry:         "gcr.io/datadoghq",
			repository:       "dd-lib-java-init",
			tag:              "latest",
			expectedImage:    "gcr.io/datadoghq/dd-lib-java-init@sha256:ghi789",
			expectedResolved: true,
		},
		"js_v5": {
			registry:         "gcr.io/datadoghq",
			repository:       "dd-lib-js-init",
			tag:              "v5",
			expectedImage:    "gcr.io/datadoghq/dd-lib-js-init@sha256:js456",
			expectedResolved: true,
		},
		"nonexistent_tag": {
			registry:         "gcr.io/datadoghq",
			repository:       "dd-lib-python-init",
			tag:              "nonexistent",
			expectedImage:    "gcr.io/datadoghq/dd-lib-python-init:nonexistent",
			expectedResolved: false,
		},
	}

	// Load the multi-repo configuration once for all test cases
	configs := loadRepositoryConfigs(t, "image_resolver_multi_repo.json")
	update := make(map[string]state.RawConfig)

	for i, config := range configs {
		configJSON, err := json.Marshal(config)
		require.NoError(t, err)
		update[fmt.Sprintf("config_%d", i)] = state.RawConfig{Config: configJSON}
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// Create resolver
			resolver := &remoteConfigImageResolver{
				imageMappings: make(map[string]map[string]ResolvedImage),
			}

			// Process the complete remote config state
			resolver.processUpdate(update, func(string, state.ApplyStatus) {})

			// Test resolution
			resolved, ok := resolver.Resolve(tc.registry, tc.repository, tc.tag)
			assert.Equal(t, tc.expectedResolved, ok, "Resolution success should match expected")

			if tc.expectedResolved {
				require.NotNil(t, resolved, "Should have resolved image when expectedResolved is true")
				assert.Equal(t, tc.expectedImage, resolved.FullImageRef, "Resolved image should match expected")
			} else {
				assert.Nil(t, resolved, "Should not have resolved image when expectedResolved is false")
			}
		})
	}
}

// TestImageResolverEmptyConfig tests the behavior with no remote config data
func TestImageResolverEmptyConfig(t *testing.T) {
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	// Process empty update (no remote config data available)
	resolver.processUpdate(map[string]state.RawConfig{}, func(string, state.ApplyStatus) {})

	// Test resolution should fail with empty cache
	resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
	assert.False(t, ok, "Resolution should fail with empty config")
	assert.Nil(t, resolved, "Should not return resolved image with empty cache")
}

func TestRemoteConfigImageResolver_Resolve(t *testing.T) {
	resolver := &remoteConfigImageResolver{
		imageMappings: map[string]map[string]ResolvedImage{
			"dd-lib-python-init": {
				"latest": {
					FullImageRef:     "gcr.io/datadoghq/dd-lib-python-init@sha256:abc123",
					Digest:           "sha256:abc123",
					CanonicalVersion: "3.0.0",
				},
				"v3": {
					FullImageRef:     "gcr.io/datadoghq/dd-lib-python-init@sha256:def456",
					Digest:           "sha256:def456",
					CanonicalVersion: "3.0.0",
				},
			},
		},
	}

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
			repository: "dd-lib-java-init",
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

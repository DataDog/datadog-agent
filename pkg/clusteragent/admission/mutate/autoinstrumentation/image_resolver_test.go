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

	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRCClient is a lightweight mock that implements RemoteConfigClient
type mockRCClient struct {
	configs     map[string]state.RawConfig
	subscribers map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))

	// For async testing
	blockGetConfigs bool
	configsReady    chan struct{}
	mu              sync.Mutex
}

// loadTestConfigFile loads a test data file and converts it to the format returned by rcClient.GetConfigs()
func loadTestConfigFile(filename string) (map[string]state.RawConfig, error) {
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file %s: %v", filename, err)
	}

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
		configs:         rawConfigs,
		subscribers:     make(map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))),
		blockGetConfigs: false,
		configsReady:    make(chan struct{}),
	}
}

func (m *mockRCClient) Subscribe(product string, _ func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
	log.Debugf("Would subscribe called with product on RCClient: %s", product)
}

func (m *mockRCClient) GetConfigs(_ string) map[string]state.RawConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.blockGetConfigs {
		<-m.configsReady // Block until unblocked
	}

	return m.configs
}

func (m *mockRCClient) setBlocking(block bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockGetConfigs = block
	if !block {
		close(m.configsReady)
	}
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
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	testConfigs, err := loadTestConfigFile("image_resolver_multi_repo.json")
	require.NoError(t, err)

	t.Run("multiple_repositories", func(t *testing.T) {
		appliedStatuses := make(map[string]state.ApplyStatus)
		applyStateCallback := func(cfgPath string, status state.ApplyStatus) {
			appliedStatuses[cfgPath] = status
		}

		resolver.processUpdate(testConfigs, applyStateCallback)

		assert.Len(t, resolver.imageMappings, 4) // python, java, js, apm-inject
		assert.Contains(t, resolver.imageMappings, "gcr.io/datadoghq/dd-lib-python-init")
		assert.Contains(t, resolver.imageMappings, "gcr.io/datadoghq/dd-lib-java-init")
		assert.Contains(t, resolver.imageMappings, "gcr.io/datadoghq/dd-lib-js-init")
		assert.Contains(t, resolver.imageMappings, "gcr.io/datadoghq/apm-inject")

		// Verify apply statuses were called
		assert.Len(t, appliedStatuses, 4)
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
			tag:            "3",
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
			tag:        "2",
			expectedOK: false,
		},
		{
			name:           "versioned_tag_with_v",
			registry:       "gcr.io/datadoghq",
			repository:     "dd-lib-python-init",
			tag:            "v3",
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:def456",
			expectedOK:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolved, ok := resolver.Resolve(tc.registry, tc.repository, tc.tag)

			if tc.expectedOK {
				assert.Eventually(t, func() bool {
					resolved, ok = resolver.Resolve(tc.registry, tc.repository, tc.tag)
					return ok
				}, 100*time.Millisecond, 5*time.Millisecond, "Should resolve after async init")

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

func TestRemoteConfigImageResolver_ErrorHandling(t *testing.T) {
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ResolvedImage),
	}

	testCases := []struct {
		name           string
		rawConfig      map[string]state.RawConfig
		expectedErrors int
		description    string
	}{
		{
			name: "invalid_json",
			rawConfig: map[string]state.RawConfig{
				"bad-config": {Config: []byte(`{invalid json}`)},
			},
			expectedErrors: 1,
			description:    "Should handle malformed JSON gracefully",
		},
		{
			name: "missing_repository_name",
			rawConfig: map[string]state.RawConfig{
				"incomplete-config": {Config: []byte(`{"repository_url": "gcr.io/test", "images": []}`)},
			},
			expectedErrors: 1,
			description:    "Should reject configs missing required fields",
		},
		{
			name: "missing_repository_url",
			rawConfig: map[string]state.RawConfig{
				"incomplete-config": {Config: []byte(`{"repository_name": "test", "images": []}`)},
			},
			expectedErrors: 1,
			description:    "Should reject configs missing repository URL",
		},
		{
			name: "images_with_missing_fields",
			rawConfig: map[string]state.RawConfig{
				"partial-images": {Config: []byte(`{
                    "repository_name": "test-repo",
                    "repository_url": "gcr.io/test",
                    "images": [
                        {"tag": "v1", "digest": ""},
                        {"tag": "", "digest": "sha256:abc"},
                        {"tag": "v2", "digest": "sha256:def"}
                    ]
                }`)},
			},
			expectedErrors: 0, // Should process valid images, skip invalid ones
			description:    "Should skip images with missing tag/digest",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var appliedStatuses []state.ApplyStatus
			resolver.processUpdate(tc.rawConfig, func(_ string, status state.ApplyStatus) {
				appliedStatuses = append(appliedStatuses, status)
			})

			errorCount := 0
			for _, status := range appliedStatuses {
				if status.State == state.ApplyStateError {
					errorCount++
				}
			}
			assert.Equal(t, tc.expectedErrors, errorCount, tc.description)
		})
	}
}

func TestRemoteConfigImageResolver_ConcurrentAccess(t *testing.T) {
	resolver := newRemoteConfigImageResolver(newMockRCClient("image_resolver_multi_repo.json")).(*remoteConfigImageResolver)

	t.Run("concurrent_read_write", func(_ *testing.T) {
		var wg sync.WaitGroup
		numReaders := 10
		numWriters := 3

		for i := 0; i < numReaders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					_, _ = resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
				}
			}()
		}

		for i := 0; i < numWriters; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					resolver.processUpdate(map[string]state.RawConfig{}, func(string, state.ApplyStatus) {})
					time.Sleep(10 * time.Millisecond)
				}
			}()
		}

		wg.Wait()
	})
}

func TestAsyncInitialization(t *testing.T) {
	t.Run("noop_during_initialization", func(t *testing.T) {
		mockClient := newMockRCClient("image_resolver_multi_repo.json")
		mockClient.setBlocking(true) // Block initialization

		resolver := newRemoteConfigImageResolverWithRetryConfig(mockClient, 2, 10*time.Millisecond)

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Should not complete image resolution during initialization")
		assert.Nil(t, resolved, "Should return nil during initialization")
	})

	t.Run("successful_async_initialization", func(t *testing.T) {
		mockClient := newMockRCClient("image_resolver_multi_repo.json")
		mockClient.setBlocking(true)

		resolver := newRemoteConfigImageResolverWithRetryConfig(mockClient, 2, 10*time.Millisecond)

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Should not complete image resolution during initialization")
		assert.Nil(t, resolved, "Should return nil during initialization")

		mockClient.setBlocking(false)

		assert.Eventually(t, func() bool {
			_, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
			return ok
		}, 100*time.Millisecond, 5*time.Millisecond, "Should resolve after async init")
	})

	t.Run("failed_initialization_stays_noop", func(t *testing.T) {
		// Empty configs cause initialization to fail
		mockClient := &mockRCClient{
			configs:         map[string]state.RawConfig{},
			subscribers:     make(map[string]func(map[string]state.RawConfig, func(string, state.ApplyStatus))),
			blockGetConfigs: false,
			configsReady:    make(chan struct{}),
		}
		close(mockClient.configsReady)

		resolver := newRemoteConfigImageResolverWithRetryConfig(mockClient, 2, 5*time.Millisecond)
		time.Sleep(50 * time.Millisecond)

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Should not complete image resolution after failed init")
		assert.Nil(t, resolved, "Should return nil after failed init")
	})
}

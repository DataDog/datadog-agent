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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	shouldBlock := m.blockGetConfigs
	channel := m.configsReady
	m.mu.Unlock()

	if shouldBlock {
		<-channel
	}

	return m.configs
}

func (m *mockRCClient) setBlocking(block bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only close when switching from blocking to non-blocking
	if !block && m.blockGetConfigs {
		close(m.configsReady)
	}

	m.blockGetConfigs = block
}

func TestNewImageResolver(t *testing.T) {
	t.Run("with_remote_config_client", func(t *testing.T) {
		mockClient := newMockRCClient("image_resolver_multi_repo.json")
		mockConfig := config.NewMock(t)
		resolver := NewImageResolver(mockClient, mockConfig)

		_, ok := resolver.(*remoteConfigImageResolver)
		assert.True(t, ok, "Should return remoteConfigImageResolver when rcClient is not nil")
	})

	t.Run("without_remote_config_client__typed_nil", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		resolver := NewImageResolver((*mockRCClient)(nil), mockConfig)

		_, ok := resolver.(*noOpImageResolver)
		assert.True(t, ok, "Should return noOpImageResolver when rcClient is nil")
	})

	t.Run("without_remote_config_client__untyped_nil", func(t *testing.T) {
		mockConfig := config.NewMock(t)
		resolver := NewImageResolver(nil, mockConfig)

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
		imageMappings: make(map[string]map[string]ImageInfo),
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
		assert.Contains(t, resolver.imageMappings, "dd-lib-python-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-java-init")
		assert.Contains(t, resolver.imageMappings, "dd-lib-js-init")
		assert.Contains(t, resolver.imageMappings, "apm-inject")

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
		imageMappings: make(map[string]map[string]ImageInfo),
	}

	resolver.processUpdate(map[string]state.RawConfig{}, func(string, state.ApplyStatus) {})

	resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
	assert.False(t, ok, "Resolution should fail with empty config")
	assert.Nil(t, resolved, "Should not return resolved image with empty cache")
}

func TestRemoteConfigImageResolver_Resolve(t *testing.T) {
	mockRCClient := newMockRCClient("image_resolver_multi_repo.json")
	datadoghqRegistries := config.NewMock(t).GetStringMap("admission_controller.auto_instrumentation.default_dd_registries")
	resolver := newRemoteConfigImageResolver(mockRCClient, datadoghqRegistries)

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
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expectedOK:     true,
		},
		{
			name:           "successful_resolution_versioned",
			registry:       "gcr.io/datadoghq",
			repository:     "dd-lib-python-init",
			tag:            "3",
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
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
			expectedResult: "gcr.io/datadoghq/dd-lib-python-init@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			expectedOK:     true,
		},
		{
			name:       "non_datadog_registry_rejected",
			registry:   "docker.io",
			repository: "dd-lib-python-init",
			tag:        "latest",
			expectedOK: false,
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
			imageMappings: make(map[string]map[string]ImageInfo),
		}
		resolved, ok := emptyResolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Resolution should fail with empty cache")
		assert.Nil(t, resolved, "Should not return resolved image with empty cache")
	})
}

func TestRemoteConfigImageResolver_ErrorHandling(t *testing.T) {
	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ImageInfo),
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

func TestRemoteConfigImageResolver_InvalidDigestValidation(t *testing.T) {
	testConfigs, err := loadTestConfigFile("invalid_digest_test.json")
	require.NoError(t, err)

	resolver := &remoteConfigImageResolver{
		imageMappings: make(map[string]map[string]ImageInfo),
	}

	resolver.updateCache(testConfigs)

	t.Run("cache_contains_only_valid_digests", func(t *testing.T) {
		resolver.mu.RLock()
		defer resolver.mu.RUnlock()

		repoCache, exists := resolver.imageMappings["dd-lib-test-digest-validation"]
		require.True(t, exists, "Repository should exist in cache after processing")

		assert.Len(t, repoCache, 1, "Cache should contain exactly 1 image with valid digest")

		for tag, imageInfo := range repoCache {
			assert.True(t, isValidDigest(imageInfo.Digest),
				"Image %s should have valid digest format, got: %s", tag, imageInfo.Digest)
		}

		assert.Contains(t, repoCache, "latest", "Should contain image with valid digest")

		// Verify specific invalid digest formats are NOT in cache
		assert.NotContains(t, repoCache, "invalid-short", "Should not contain image with short digest")
		assert.NotContains(t, repoCache, "missing-prefix", "Should not contain image missing sha256: prefix")
		assert.NotContains(t, repoCache, "invalid-algorithm", "Should not contain image with unsupported algorithm")
		assert.NotContains(t, repoCache, "malformed", "Should not contain image with malformed digest")
		assert.NotContains(t, repoCache, "empty", "Should not contain image with empty digest")
	})
}

func TestRemoteConfigImageResolver_ConcurrentAccess(t *testing.T) {
	datadoghqRegistries := config.NewMock(t).GetStringMap("admission_controller.auto_instrumentation.default_dd_registries")
	resolver := newRemoteConfigImageResolver(newMockRCClient("image_resolver_multi_repo.json"), datadoghqRegistries).(*remoteConfigImageResolver)

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

func TestIsDatadoghqRegistry(t *testing.T) {
	testCases := []struct {
		name     string
		registry string
		expected bool
	}{
		{
			name:     "gcr_io_datadoghq",
			registry: "gcr.io/datadoghq",
			expected: true,
		},
		{
			name:     "hub_docker_com_datadog",
			registry: "docker.io/datadog",
			expected: true,
		},
		{
			name:     "gallery_ecr_aws_datadog",
			registry: "public.ecr.aws/datadog",
			expected: true,
		},
		{
			name:     "docker_io_not_datadog",
			registry: "docker.io",
			expected: false,
		},
		{
			name:     "empty_registry",
			registry: "",
			expected: false,
		},
	}

	for _, tc := range testCases {

		t.Run(tc.name, func(t *testing.T) {
			mockConfig := config.NewMock(t)
			result := isDatadoghqRegistry(tc.registry, mockConfig.GetStringMap("admission_controller.auto_instrumentation.default_dd_registries"))
			assert.Equal(t, tc.expected, result, "isDatadoghqRegistry(%s) should return %v", tc.registry, tc.expected)
		})
	}
}

func TestAsyncInitialization(t *testing.T) {
	t.Run("noop_during_initialization", func(t *testing.T) {
		mockClient := newMockRCClient("image_resolver_multi_repo.json")
		mockClient.setBlocking(true) // Block initialization
		config.NewMock(t).GetStringMap("admission_controller.auto_instrumentation.default_dd_registries")

		resolver := newRemoteConfigImageResolverWithRetryConfig(
			mockClient,
			2,
			10*time.Millisecond,
			config.NewMock(t).GetStringMap("admission_controller.auto_instrumentation.default_dd_registries"),
		)

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Should not complete image resolution during initialization")
		assert.Nil(t, resolved, "Should return nil during initialization")
	})

	t.Run("successful_async_initialization", func(t *testing.T) {
		mockClient := newMockRCClient("image_resolver_multi_repo.json")
		mockClient.setBlocking(true)

		resolver := newRemoteConfigImageResolverWithRetryConfig(
			mockClient,
			2,
			10*time.Millisecond,
			config.NewMock(t).GetStringMap("admission_controller.auto_instrumentation.default_dd_registries"),
		)

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

		resolver := newRemoteConfigImageResolverWithRetryConfig(
			mockClient,
			2,
			10*time.Millisecond,
			config.NewMock(t).GetStringMap("admission_controller.auto_instrumentation.default_dd_registries"),
		)
		time.Sleep(50 * time.Millisecond)

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "latest")
		assert.False(t, ok, "Should not complete image resolution after failed init")
		assert.Nil(t, resolved, "Should return nil after failed init")
	})
}

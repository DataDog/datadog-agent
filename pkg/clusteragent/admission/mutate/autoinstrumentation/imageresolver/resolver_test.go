// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"net/http"
	"testing"

	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestNew(t *testing.T) {
	t.Run("gradual_rollout_enabled", func(t *testing.T) {
		mockConfig := NewConfig(config.NewMock(t))
		resolver := New(mockConfig)

		_, ok := resolver.(*bucketTagResolver)
		assert.True(t, ok, "Should return bucketTagResolver when gradual rollout is enabled")
	})

	t.Run("gradual_rollout_disabled", func(t *testing.T) {
		mockConfig := Config{
			Enabled: false,
		}
		mockConfig.Enabled = false
		resolver := New(mockConfig)

		_, ok := resolver.(*noOpResolver)
		assert.True(t, ok, "Should return noOpImageResolver when gradual rollout is disabled")
	})
}

func TestNoOpResolver_Resolve(t *testing.T) {
	resolver := NewNoOpResolver()

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
			datadogRegistries := newDatadoghqRegistries(mockConfig.GetStringSlice("admission_controller.auto_instrumentation.default_dd_registries"))
			result := isDatadoghqRegistry(tc.registry, datadogRegistries)
			assert.Equal(t, tc.expected, result, "isDatadoghqRegistry(%s) should return %v", tc.registry, tc.expected)
		})
	}
}

func newMockBucketTagResolver(ttl time.Duration, bucketID string) bucketTagResolver {
	datadogRegistries := map[string]struct{}{
		"gcr.io/datadoghq":       {},
		"public.ecr.aws/datadog": {},
	}
	transport := &mockRoundTripper{
		responses: make(map[string]*http.Response),
	}
	return bucketTagResolver{
		cache:               newHTTPDigestCache(ttl, datadogRegistries, transport),
		bucketID:            bucketID,
		datadoghqRegistries: datadogRegistries,
	}
}

func TestBucketTagResolver_Resolve(t *testing.T) {
	t.Run("rejects_non_datadog_registry", func(t *testing.T) {
		resolver := newMockBucketTagResolver(5*time.Minute, "3")

		resolved, ok := resolver.Resolve("docker.io", "library/nginx", "latest")

		assert.False(t, ok, "Should reject non-Datadog registry")
		assert.Nil(t, resolved, "Should return nil for non-Datadog registry")
	})

	t.Run("accepts_allowed_registries", func(t *testing.T) {
		resolver := newMockBucketTagResolver(5*time.Minute, "4")

		resolver.cache.cache["gcr.io/datadoghq"] = repositoryCache{
			"dd-lib-java-init": tagCache{
				"1-gr4": {
					digest:     "sha256:abc123",
					whenCached: time.Now(),
				},
			},
		}
		resolver.cache.cache["public.ecr.aws/datadog"] = repositoryCache{
			"dd-lib-java-init": tagCache{
				"1-gr4": {
					digest:     "sha256:abc123",
					whenCached: time.Now(),
				},
			},
		}

		testCases := []struct {
			registry string
		}{
			{"gcr.io/datadoghq"},
			{"public.ecr.aws/datadog"},
		}

		for _, tc := range testCases {
			t.Run(tc.registry, func(t *testing.T) {
				resolved, ok := resolver.Resolve(tc.registry, "dd-lib-java-init", "v1")
				assert.True(t, ok, "Should accept allowed registry")
				assert.NotNil(t, resolved, "Should return resolved image")
			})
		}
	})

	t.Run("cache_hit_returns_resolved_image", func(t *testing.T) {
		resolver := newMockBucketTagResolver(5*time.Minute, "1")

		resolver.cache.cache["gcr.io/datadoghq"] = repositoryCache{
			"dd-lib-python-init": tagCache{
				"3-gr1": {
					digest:     "sha256:def456",
					whenCached: time.Now(),
				},
			},
		}

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-python-init", "v3")

		assert.True(t, ok, "Should resolve cached image")
		require.NotNil(t, resolved, "Should return resolved image")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-python-init@sha256:def456", resolved.FullImageRef)
		assert.Equal(t, "v3", resolved.CanonicalVersion)
	})

	t.Run("cache_miss_returns_nil", func(t *testing.T) {
		resolver := newMockBucketTagResolver(5*time.Minute, "2")

		resolved, ok := resolver.Resolve("gcr.io/datadoghq", "dd-lib-java-init", "v1")

		assert.False(t, ok, "Should return false on cache miss")
		assert.Nil(t, resolved, "Should return nil on cache miss")
	})

	t.Run("v_prefix_normalization_for_major_versions", func(t *testing.T) {
		resolver := newMockBucketTagResolver(5*time.Minute, "3")

		resolver.cache.cache["gcr.io/datadoghq"] = repositoryCache{
			"dd-lib-js-init": tagCache{
				"2-gr3": {
					digest:     "sha256:xyz789",
					whenCached: time.Now(),
				},
			},
		}

		resolved1, ok1 := resolver.Resolve("gcr.io/datadoghq", "dd-lib-js-init", "v2")
		resolved2, ok2 := resolver.Resolve("gcr.io/datadoghq", "dd-lib-js-init", "2")

		require.True(t, ok1, "Should resolve v2")
		require.True(t, ok2, "Should resolve 2")
		assert.Equal(t, resolved1.FullImageRef, resolved2.FullImageRef, "Both tags should resolve to same image")
	})

	t.Run("canonical_versions_not_bucket_tagged", func(t *testing.T) {
		resolver := newMockBucketTagResolver(5*time.Minute, "5")

		resolver.cache.cache["gcr.io/datadoghq"] = repositoryCache{
			"dd-lib-ruby-init": tagCache{
				"1.2.3": {
					digest:     "sha256:canonical",
					whenCached: time.Now(),
				},
			},
		}

		resolved1, ok1 := resolver.Resolve("gcr.io/datadoghq", "dd-lib-ruby-init", "v1.2.3")
		require.True(t, ok1, "Should resolve v1.2.3")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-ruby-init@sha256:canonical", resolved1.FullImageRef)
		resolved2, ok2 := resolver.Resolve("gcr.io/datadoghq", "dd-lib-ruby-init", "1.2.3")
		assert.True(t, ok2, "Should resolve 1.2.3 (same as v1.2.3)")
		assert.Equal(t, "gcr.io/datadoghq/dd-lib-ruby-init@sha256:canonical", resolved2.FullImageRef)
	})
}

func TestBucketTagResolver_CreateBucketTag(t *testing.T) {
	testCases := []struct {
		name     string
		bucketID string
		inputTag string
		expected string
	}{
		{
			name:     "major_version_with_v_prefix",
			bucketID: "2",
			inputTag: "v3",
			expected: "3-gr2",
		},
		{
			name:     "major_version_without_v_prefix",
			bucketID: "9",
			inputTag: "1",
			expected: "1-gr9",
		},
		{
			name:     "latest_tag_gets_bucket",
			bucketID: "3",
			inputTag: "latest",
			expected: "latest-gr3",
		},
		{
			name:     "minor_version_with_v_prefix_normalized",
			bucketID: "4",
			inputTag: "v1.0",
			expected: "1.0",
		},
		{
			name:     "minor_version_without_v_prefix_unchanged",
			bucketID: "5",
			inputTag: "4.2",
			expected: "4.2",
		},
		{
			name:     "canonical_version_with_v_normalized",
			bucketID: "6",
			inputTag: "v1.2.3",
			expected: "1.2.3",
		},
		{
			name:     "canonical_version_without_v_unchanged",
			bucketID: "7",
			inputTag: "2.3.4",
			expected: "2.3.4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockConfig := Config{
				DigestCacheTTL: 5 * time.Minute,
				BucketID:       tc.bucketID,
				DDRegistries:   map[string]struct{}{"gcr.io/datadoghq": {}},
			}
			resolver := newBucketTagResolver(mockConfig)
			result := resolver.createBucketTag(tc.inputTag)
			assert.Equal(t, tc.expected, result)
		})
	}
}

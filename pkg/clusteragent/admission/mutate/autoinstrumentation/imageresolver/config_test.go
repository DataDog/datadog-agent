// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"fmt"
	"math"
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
				BucketID:       "2",
				DigestCacheTTL: 1 * time.Hour,
				Enabled:        true,
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
				BucketID:       "2",
				DigestCacheTTL: 1 * time.Hour,
				Enabled:        true,
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
				BucketID:       "2",
				DigestCacheTTL: 1 * time.Hour,
				Enabled:        true,
			},
		},
		{
			name: "bucket_id_based_on_api_key",
			configFactory: func(t *testing.T) config.Component {
				mockConfig := config.NewMock(t)
				mockConfig.SetWithoutSource("site", "datadoghq.com")
				mockConfig.SetWithoutSource("api_key", "1234567890abcdef")
				return mockConfig
			},
			expectedState: Config{
				Site:           "datadoghq.com",
				DDRegistries:   map[string]struct{}{"gcr.io/datadoghq": {}, "docker.io/datadog": {}, "public.ecr.aws/datadog": {}},
				BucketID:       "0",
				DigestCacheTTL: 1 * time.Hour,
				Enabled:        true,
			},
		},
		{
			name: "gradual_rollout_disabled",
			configFactory: func(t *testing.T) config.Component {
				mockConfig := config.NewMock(t)
				mockConfig.SetWithoutSource("site", "datadoghq.com")
				mockConfig.SetWithoutSource("api_key", "1234567890abcdef")
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.gradual_rollout.enabled", false)
				return mockConfig
			},
			expectedState: Config{
				Site:           "datadoghq.com",
				DDRegistries:   map[string]struct{}{"gcr.io/datadoghq": {}, "docker.io/datadog": {}, "public.ecr.aws/datadog": {}},
				BucketID:       "0",
				DigestCacheTTL: 1 * time.Hour,
				Enabled:        false,
			},
		},
		{
			name: "gradual_rollout_cache_ttl_hours_configured",
			configFactory: func(t *testing.T) config.Component {
				mockConfig := config.NewMock(t)
				mockConfig.SetWithoutSource("site", "datadoghq.com")
				mockConfig.SetWithoutSource("api_key", "1234567890abcdef")
				mockConfig.SetWithoutSource("admission_controller.auto_instrumentation.gradual_rollout.cache_ttl", "2h")
				return mockConfig
			},
			expectedState: Config{
				Site:           "datadoghq.com",
				DDRegistries:   map[string]struct{}{"gcr.io/datadoghq": {}, "docker.io/datadog": {}, "public.ecr.aws/datadog": {}},
				BucketID:       "0",
				DigestCacheTTL: 2 * time.Hour,
				Enabled:        true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := tt.configFactory(t)
			result := NewConfig(mockConfig)

			require.Equal(t, tt.expectedState, result)
		})
	}
}

func TestCalculateRolloutBucket_EvenlyDistributed(t *testing.T) {
	bucketCounts := make(map[string]int)

	numSamples := 10000
	for i := 0; i < numSamples; i++ {
		apiKey := fmt.Sprintf("api-key-%d", i)
		bucket := calculateRolloutBucket(apiKey)
		bucketCounts[bucket]++
	}

	require.Len(t, bucketCounts, rolloutBucketCount, "Should use all %d buckets", rolloutBucketCount)

	expectedPerBucket := float64(numSamples) / float64(rolloutBucketCount)
	p := 1.0 / float64(rolloutBucketCount)
	stdDev := math.Sqrt(float64(numSamples) * p * (1.0 - p))
	tolerance := 4.0 // 4 std devs give 99.99% confidence

	minCount := int(expectedPerBucket - tolerance*stdDev)
	maxCount := int(expectedPerBucket + tolerance*stdDev)

	for bucket, count := range bucketCounts {
		require.GreaterOrEqual(t, count, minCount,
			"Bucket %s has too few samples: %d (expected between %d and %d)",
			bucket, count, minCount, maxCount)
		require.LessOrEqual(t, count, maxCount,
			"Bucket %s has too many samples: %d (expected between %d and %d)",
			bucket, count, minCount, maxCount)
	}
}

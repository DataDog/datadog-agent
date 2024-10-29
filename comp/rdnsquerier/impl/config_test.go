// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	var tests = []struct {
		name           string
		configYaml     string
		expectedConfig rdnsQuerierConfig
	}{
		{
			name:       "disabled by default",
			configYaml: ``,
			expectedConfig: rdnsQuerierConfig{
				enabled:  false,
				workers:  0,
				chanSize: 0,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        0,
					cleanInterval:   0,
					persistInterval: 0,
					maxRetries:      -1,
					maxSize:         0,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            0,
					limitThrottledPerSec:   0,
					throttleErrorThreshold: 0,
					recoveryIntervals:      0,
					recoveryInterval:       0,
				},
			},
		},
		{
			name: "disabled when Network Path Collector is enabled, reverse DNS enrichment is disabled",
			configYaml: `
network_path:
  connections_monitoring:
    enabled: true
  collector:
    reverse_dns_enrichment:
      enabled: false
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  false,
				workers:  0,
				chanSize: 0,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        0,
					cleanInterval:   0,
					persistInterval: 0,
					maxRetries:      -1,
					maxSize:         0,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            0,
					limitThrottledPerSec:   0,
					throttleErrorThreshold: 0,
					recoveryIntervals:      0,
					recoveryInterval:       0,
				},
			},
		},
		{
			name: "default config when enabled through netflow",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  defaultWorkers,
				chanSize: defaultChanSize,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        defaultCacheEntryTTL,
					cleanInterval:   defaultCacheCleanInterval,
					persistInterval: defaultCachePersistInterval,
					maxRetries:      defaultCacheMaxRetries,
					maxSize:         defaultCacheMaxSize,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            defaultRateLimitPerSec,
					limitThrottledPerSec:   defaultRateLimitThrottledPerSec,
					throttleErrorThreshold: defaultRateLimitThrottleErrorThreshold,
					recoveryIntervals:      defaultRateLimitRecoveryIntervals,
					recoveryInterval:       defaultRateLimitRecoveryInterval,
				},
			},
		},
		{
			name: "default config when Network Path Collector is enabled",
			configYaml: `
network_path:
  connections_monitoring:
    enabled: true
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  defaultWorkers,
				chanSize: defaultChanSize,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        defaultCacheEntryTTL,
					cleanInterval:   defaultCacheCleanInterval,
					persistInterval: defaultCachePersistInterval,
					maxRetries:      defaultCacheMaxRetries,
					maxSize:         defaultCacheMaxSize,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            defaultRateLimitPerSec,
					limitThrottledPerSec:   defaultRateLimitThrottledPerSec,
					throttleErrorThreshold: defaultRateLimitThrottleErrorThreshold,
					recoveryIntervals:      defaultRateLimitRecoveryIntervals,
					recoveryInterval:       defaultRateLimitRecoveryInterval,
				},
			},
		},
		{
			name: "use defaults for invalid values",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
reverse_dns_enrichment:
  workers: 0
  chan_size: 0
  cache:
    enabled: true
    entry_ttl: 0
    clean_interval: 0
    persist_interval: 0
    max_retries: -1
    max_size: 0
  rate_limiter:
    enabled: true
    limit_per_sec: 0
    limit_throttled_per_sec: 0
    throttle_error_threshold: 0
    recovery_intervals : 0
    recovery_interval: 0
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  defaultWorkers,
				chanSize: defaultChanSize,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        defaultCacheEntryTTL,
					cleanInterval:   defaultCacheCleanInterval,
					persistInterval: defaultCachePersistInterval,
					maxRetries:      defaultCacheMaxRetries,
					maxSize:         defaultCacheMaxSize,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            defaultRateLimitPerSec,
					limitThrottledPerSec:   defaultRateLimitThrottledPerSec,
					throttleErrorThreshold: defaultRateLimitThrottleErrorThreshold,
					recoveryIntervals:      defaultRateLimitRecoveryIntervals,
					recoveryInterval:       defaultRateLimitRecoveryInterval,
				},
			},
		},
		{
			name: "specific config",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
reverse_dns_enrichment:
  workers: 25
  chan_size: 999
  cache:
    enabled: true
    entry_ttl: 24h
    clean_interval: 30m
    persist_interval: 2h
    max_retries: 1
    max_size: 100_000
  rate_limiter:
    enabled: true
    limit_per_sec: 111
    limit_throttled_per_sec: 5
    throttle_error_threshold: 11
    recovery_intervals : 10
    recovery_interval: 10s
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  25,
				chanSize: 999,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        24 * time.Hour,
					cleanInterval:   30 * time.Minute,
					persistInterval: 2 * time.Hour,
					maxRetries:      1,
					maxSize:         100_000,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            111,
					limitThrottledPerSec:   5,
					throttleErrorThreshold: 11,
					recoveryIntervals:      10,
					recoveryInterval:       10 * time.Second,
				},
			},
		},
		{
			name: "specific config with defaults when rate limiter and cache are enabled",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
reverse_dns_enrichment:
  workers: 25
  chan_size: 999
  cache:
    enabled: true
  rate_limiter:
    enabled: true
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  25,
				chanSize: 999,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        defaultCacheEntryTTL,
					cleanInterval:   defaultCacheCleanInterval,
					persistInterval: defaultCachePersistInterval,
					maxRetries:      defaultCacheMaxRetries,
					maxSize:         defaultCacheMaxSize,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            defaultRateLimitPerSec,
					limitThrottledPerSec:   defaultRateLimitThrottledPerSec,
					throttleErrorThreshold: defaultRateLimitThrottleErrorThreshold,
					recoveryIntervals:      defaultRateLimitRecoveryIntervals,
					recoveryInterval:       defaultRateLimitRecoveryInterval,
				},
			},
		},
		{
			name: "specific config with rate limiter and cache disabled",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
reverse_dns_enrichment:
  workers: 25
  chan_size: 999
  cache:
    enabled: false
  rate_limiter:
    enabled: false
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  25,
				chanSize: 999,
				cache: cacheConfig{
					enabled:         false,
					entryTTL:        0,
					cleanInterval:   0,
					persistInterval: 0,
					maxRetries:      -1,
					maxSize:         0,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                false,
					limitPerSec:            0,
					limitThrottledPerSec:   0,
					throttleErrorThreshold: 0,
					recoveryIntervals:      0,
					recoveryInterval:       0,
				},
			},
		},
		{
			name: "rate_limiter limit_throttled_per_sec > limit_per_sec",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
reverse_dns_enrichment:
  workers: 25
  chan_size: 999
  rate_limiter:
    enabled: true
    limit_per_sec: 50
    limit_throttled_per_sec: 500
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:  true,
				workers:  25,
				chanSize: 999,
				cache: cacheConfig{
					enabled:         true,
					entryTTL:        defaultCacheEntryTTL,
					cleanInterval:   defaultCacheCleanInterval,
					persistInterval: defaultCachePersistInterval,
					maxRetries:      defaultCacheMaxRetries,
					maxSize:         defaultCacheMaxSize,
				},
				rateLimiter: rateLimiterConfig{
					enabled:                true,
					limitPerSec:            50,
					limitThrottledPerSec:   50,
					throttleErrorThreshold: defaultRateLimitThrottleErrorThreshold,
					recoveryIntervals:      defaultRateLimitRecoveryIntervals,
					recoveryInterval:       defaultRateLimitRecoveryInterval,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := mock.NewFromYAML(t, tt.configYaml)
			testConfig := newConfig(mockConfig)
			assert.Equal(t, tt.expectedConfig, *testConfig)
		})
	}
}

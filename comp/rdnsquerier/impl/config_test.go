// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				enabled:              false,
				workers:              0,
				chanSize:             0,
				rateLimiterEnabled:   true,
				rateLimitPerSec:      0,
				cacheEnabled:         true,
				cacheEntryTTL:        0,
				cacheCleanInterval:   0,
				cachePersistInterval: 0,
			},
		},
		{
			name: "default config when enabled",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:              true,
				workers:              defaultWorkers,
				chanSize:             defaultChanSize,
				rateLimiterEnabled:   true,
				rateLimitPerSec:      defaultRateLimitPerSec,
				cacheEnabled:         true,
				cacheEntryTTL:        defaultCacheEntryTTL,
				cacheCleanInterval:   defaultCacheCleanInterval,
				cachePersistInterval: defaultCachePersistInterval,
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
  rate_limiter:
    enabled: true
    limit_per_sec: 111
  cache:
    enabled: true
    entry_ttl: 222
    clean_interval: 333
    persist_interval: 444
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:              true,
				workers:              25,
				chanSize:             999,
				rateLimiterEnabled:   true,
				rateLimitPerSec:      111,
				cacheEnabled:         true,
				cacheEntryTTL:        222,
				cacheCleanInterval:   333,
				cachePersistInterval: 444,
			},
		},
		{
			name: "specific config with rate limiter and cache enabled",
			configYaml: `
network_devices:
  netflow:
    reverse_dns_enrichment_enabled: true
reverse_dns_enrichment:
  workers: 25
  chan_size: 999
  rate_limiter:
    enabled: true
  cache:
    enabled: true
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:              true,
				workers:              25,
				chanSize:             999,
				rateLimiterEnabled:   true,
				rateLimitPerSec:      defaultRateLimitPerSec,
				cacheEnabled:         true,
				cacheEntryTTL:        defaultCacheEntryTTL,
				cacheCleanInterval:   defaultCacheCleanInterval,
				cachePersistInterval: defaultCachePersistInterval,
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
  rate_limiter:
    enabled: false
  cache:
    enabled: false
`,
			expectedConfig: rdnsQuerierConfig{
				enabled:              true,
				workers:              25,
				chanSize:             999,
				rateLimiterEnabled:   false,
				rateLimitPerSec:      0,
				cacheEnabled:         false,
				cacheEntryTTL:        0,
				cacheCleanInterval:   0,
				cachePersistInterval: 0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.Datadog().SetConfigType("yaml")
			err := config.Datadog().ReadConfig(strings.NewReader(tt.configYaml))
			require.NoError(t, err)

			testConfig := newConfig(config.Datadog())
			assert.Equal(t, tt.expectedConfig, *testConfig)
		})
	}
}

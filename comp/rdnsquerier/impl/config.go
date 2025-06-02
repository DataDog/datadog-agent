// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

type rdnsQuerierConfig struct {
	enabled  bool
	workers  int
	chanSize int

	cache       cacheConfig
	rateLimiter rateLimiterConfig
}

type cacheConfig struct {
	enabled         bool
	entryTTL        time.Duration
	cleanInterval   time.Duration
	persistInterval time.Duration
	maxRetries      int
	maxSize         int
}

type rateLimiterConfig struct {
	enabled                bool
	limitPerSec            int
	limitThrottledPerSec   int
	throttleErrorThreshold int
	recoveryIntervals      int
	recoveryInterval       time.Duration
}

const (
	defaultWorkers  = 10
	defaultChanSize = 5000

	defaultCacheEntryTTL        = 24 * time.Hour
	defaultCacheCleanInterval   = 2 * time.Hour
	defaultCachePersistInterval = 2 * time.Hour
	defaultCacheMaxRetries      = 10
	defaultCacheMaxSize         = 1_000_000

	defaultRateLimitPerSec                 = 1000
	defaultRateLimitThrottledPerSec        = 1
	defaultRateLimitThrottleErrorThreshold = 10
	defaultRateLimitRecoveryIntervals      = 5
	defaultRateLimitRecoveryInterval       = 5 * time.Second
)

func newConfig(agentConfig config.Component) *rdnsQuerierConfig {
	netflowRDNSEnrichmentEnabled := agentConfig.GetBool("network_devices.netflow.reverse_dns_enrichment_enabled")
	networkPathRDNSEnrichmentEnabled := agentConfig.GetBool("network_path.collector.reverse_dns_enrichment.enabled") && agentConfig.GetBool("network_path.connections_monitoring.enabled")

	c := &rdnsQuerierConfig{
		enabled:  netflowRDNSEnrichmentEnabled || networkPathRDNSEnrichmentEnabled,
		workers:  agentConfig.GetInt("reverse_dns_enrichment.workers"),
		chanSize: agentConfig.GetInt("reverse_dns_enrichment.chan_size"),

		cache: cacheConfig{
			enabled:         agentConfig.GetBool("reverse_dns_enrichment.cache.enabled"),
			entryTTL:        agentConfig.GetDuration("reverse_dns_enrichment.cache.entry_ttl"),
			cleanInterval:   agentConfig.GetDuration("reverse_dns_enrichment.cache.clean_interval"),
			persistInterval: agentConfig.GetDuration("reverse_dns_enrichment.cache.persist_interval"),
			maxRetries:      agentConfig.GetInt("reverse_dns_enrichment.cache.max_retries"),
			maxSize:         agentConfig.GetInt("reverse_dns_enrichment.cache.max_size"),
		},

		rateLimiter: rateLimiterConfig{
			enabled:                agentConfig.GetBool("reverse_dns_enrichment.rate_limiter.enabled"),
			limitPerSec:            agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.limit_per_sec"),
			limitThrottledPerSec:   agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.limit_throttled_per_sec"),
			throttleErrorThreshold: agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.throttle_error_threshold"),
			recoveryIntervals:      agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.recovery_intervals"),
			recoveryInterval:       agentConfig.GetDuration("reverse_dns_enrichment.rate_limiter.recovery_interval"),
		},
	}

	c.setDefaults()
	return c
}

func (c *rdnsQuerierConfig) setDefaults() {
	if !c.enabled {
		return
	}

	if c.workers <= 0 {
		c.workers = defaultWorkers
	}

	if c.chanSize <= 0 {
		c.chanSize = defaultChanSize
	}

	if c.cache.enabled {
		if c.cache.entryTTL <= 0 {
			c.cache.entryTTL = defaultCacheEntryTTL
		}
		if c.cache.cleanInterval <= 0 {
			c.cache.cleanInterval = defaultCacheCleanInterval
		}
		if c.cache.persistInterval <= 0 {
			c.cache.persistInterval = defaultCachePersistInterval
		}
		if c.cache.maxRetries < 0 {
			c.cache.maxRetries = defaultCacheMaxRetries
		}
		if c.cache.maxSize <= 0 {
			c.cache.maxSize = defaultCacheMaxSize
		}
	}

	if c.rateLimiter.enabled {
		if c.rateLimiter.limitPerSec <= 0 {
			c.rateLimiter.limitPerSec = defaultRateLimitPerSec
		}
		if c.rateLimiter.limitThrottledPerSec <= 0 {
			c.rateLimiter.limitThrottledPerSec = defaultRateLimitThrottledPerSec
		}
		if c.rateLimiter.limitThrottledPerSec > c.rateLimiter.limitPerSec {
			c.rateLimiter.limitThrottledPerSec = c.rateLimiter.limitPerSec
		}
		if c.rateLimiter.throttleErrorThreshold <= 0 {
			c.rateLimiter.throttleErrorThreshold = defaultRateLimitThrottleErrorThreshold
		}
		if c.rateLimiter.recoveryIntervals <= 0 {
			c.rateLimiter.recoveryIntervals = defaultRateLimitRecoveryIntervals
		}
		if c.rateLimiter.recoveryInterval <= 0 {
			c.rateLimiter.recoveryInterval = defaultRateLimitRecoveryInterval
		}
	}
}

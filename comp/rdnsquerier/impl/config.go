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

	rateLimiterEnabled bool
	rateLimitPerSec    int

	cacheEnabled         bool
	cacheEntryTTL        time.Duration
	cacheCleanInterval   time.Duration
	cachePersistInterval time.Duration
}

const (
	defaultWorkers  = 10
	defaultChanSize = 1000

	defaultRateLimitPerSec = 1000

	defaultCacheEntryTTL        = time.Hour
	defaultCacheCleanInterval   = 30 * time.Minute
	defaultCachePersistInterval = 30 * time.Minute
)

func newConfig(agentConfig config.Component) *rdnsQuerierConfig {
	netflowRDNSEnrichmentEnabled := agentConfig.GetBool("network_devices.netflow.reverse_dns_enrichment_enabled")

	c := &rdnsQuerierConfig{
		enabled:  netflowRDNSEnrichmentEnabled,
		workers:  agentConfig.GetInt("reverse_dns_enrichment.workers"),
		chanSize: agentConfig.GetInt("reverse_dns_enrichment.chan_size"),

		rateLimiterEnabled: agentConfig.GetBool("reverse_dns_enrichment.rate_limiter.enabled"),
		rateLimitPerSec:    agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.limit_per_sec"),

		cacheEnabled:         agentConfig.GetBool("reverse_dns_enrichment.cache.enabled"),
		cacheEntryTTL:        agentConfig.GetDuration("reverse_dns_enrichment.cache.entry_ttl"),
		cacheCleanInterval:   agentConfig.GetDuration("reverse_dns_enrichment.cache.clean_interval"),
		cachePersistInterval: agentConfig.GetDuration("reverse_dns_enrichment.cache.persist_interval"),
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

	if c.rateLimiterEnabled {
		if c.rateLimitPerSec <= 0 {
			c.rateLimitPerSec = defaultRateLimitPerSec
		}
	}

	if c.cacheEnabled {
		if c.cacheEntryTTL <= 0 {
			c.cacheEntryTTL = defaultCacheEntryTTL
		}
		if c.cacheCleanInterval <= 0 {
			c.cacheCleanInterval = defaultCacheCleanInterval
		}
		if c.cachePersistInterval <= 0 {
			c.cachePersistInterval = defaultCachePersistInterval
		}
	}
}

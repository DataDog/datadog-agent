// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
)

type rdnsQuerierConfig struct {
	enabled  bool
	workers  int
	chanSize int

	rateLimiterEnabled bool
	rateLimiterLimit   int
	rateLimiterBurst   int

	cacheEnabled         bool
	cacheEntryTTL        int
	cacheCleanInterval   int
	cachePersistInterval int

	// debug - TODO remove
	fakeResolver                 bool
	generateFakeQueriesPerSecond int
	lookupDelayMs                int
}

func newConfig(agentConfig config.Component, logger log.Component) *rdnsQuerierConfig {
	netflowRDNSEnrichmentEnabled := agentConfig.GetBool("network_devices.netflow.reverse_dns_enrichment_enabled")

	c := &rdnsQuerierConfig{
		enabled:  netflowRDNSEnrichmentEnabled,
		workers:  agentConfig.GetInt("reverse_dns_enrichment.workers"),
		chanSize: agentConfig.GetInt("reverse_dns_enrichment.chan_size"),

		rateLimiterEnabled: agentConfig.GetBool("reverse_dns_enrichment.rate_limiter.enabled"),
		rateLimiterLimit:   agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.limit"),
		rateLimiterBurst:   agentConfig.GetInt("reverse_dns_enrichment.rate_limiter.burst"),

		cacheEnabled:         agentConfig.GetBool("reverse_dns_enrichment.cache.enabled"),
		cacheEntryTTL:        agentConfig.GetInt("reverse_dns_enrichment.cache.entry_ttl"),
		cacheCleanInterval:   agentConfig.GetInt("reverse_dns_enrichment.cache.clean_interval"),
		cachePersistInterval: agentConfig.GetInt("reverse_dns_enrichment.cache.persist_interval"),

		fakeResolver:                 agentConfig.GetBool("reverse_dns_enrichment.fake_resolver"),
		generateFakeQueriesPerSecond: agentConfig.GetInt("reverse_dns_enrichment.generate_fake_queries_per_second"),
		lookupDelayMs:                agentConfig.GetInt("reverse_dns_enrichment.lookup_delay_ms"),
	}

	c.validateConfig(logger)

	return c
}

func (c *rdnsQuerierConfig) validateConfig(logger log.Component) {
	if c.enabled {
		logger.Infof("Reverse DNS Enrichment component is enabled")
	} else {
		logger.Infof("Reverse DNS Enrichment component is disabled")
		return
	}

	if c.workers <= 0 {
		logger.Warnf("Reverse DNS Enrichment: Invalid number of workers %d, setting to 1", c.workers)
		c.workers = 1
	}

	if c.chanSize < 0 {
		logger.Warnf("Reverse DNS Enrichment: Invalid channel size %d, setting to 0 (unbuffered)", c.chanSize)
		c.chanSize = 0
	}

	if c.rateLimiterEnabled {
		if c.rateLimiterLimit <= 0 {
			logger.Warnf("Reverse DNS Enrichment: Invalid rate limiter limit %d, setting to 1000", c.rateLimiterLimit)
			c.rateLimiterLimit = 1000
		}
		if c.rateLimiterBurst < 0 {
			logger.Warnf("Reverse DNS Enrichment: Invalid rate limiter burst %d, setting to 1", c.rateLimiterBurst)
			c.rateLimiterBurst = 1
		}
	}

	if c.cacheEnabled {
		if c.cacheEntryTTL <= 0 {
			logger.Warnf("Reverse DNS Enrichment: Invalid cache entry TTL, setting to 60 minutes")
			c.cacheEntryTTL = 60 * 60
		}
		if c.cacheCleanInterval <= 0 {
			logger.Warnf("Reverse DNS Enrichment: Invalid cache clean interval, setting to 30 minutes")
			c.cacheCleanInterval = 30 * 60
		}
		if c.cachePersistInterval <= 0 {
			logger.Warnf("Reverse DNS Enrichment: Invalid cache persist interval, setting to 30 minutes")
			c.cachePersistInterval = 30 * 60
		}
	}
}

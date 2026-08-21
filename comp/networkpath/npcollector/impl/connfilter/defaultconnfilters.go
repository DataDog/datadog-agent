// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connfilter

// getDefaultConnFilters returns the default connection filters.
func getDefaultConnFilters(site string, monitorIPWithoutDomain bool) []Config {
	defaultConfig := []Config{
		{
			Type:        FilterTypeExclude,
			MatchDomain: "*.datadog.pool.ntp.org",
		},
		{
			Type:        FilterTypeExclude,
			MatchDomain: "*.datadoghq.com",
		},
		{
			Type:        FilterTypeExclude,
			MatchDomain: "*.datadoghq.eu",
		},
		{
			Type:        FilterTypeExclude,
			MatchDomain: "*.local",
		},
		{
			Type:        FilterTypeExclude,
			MatchDomain: "*.internal",
		},
		// Datadog intake endpoints are CNAME'd to these AWS ELBs. Exclude the
		// Network Path-specific load balancer names when the original Datadog
		// domain is no longer present in the shared reverse-DNS cache.
		{
			Type:        FilterTypeExclude,
			MatchDomain: "l4-metrics-agent-*.elb.*.amazonaws.com",
		},
	}
	if site != "" {
		defaultConfig = append(defaultConfig, Config{
			Type:        FilterTypeExclude,
			MatchDomain: "*." + site,
		})
	}
	if monitorIPWithoutDomain {
		defaultConfig = append(defaultConfig, Config{
			Type:    FilterTypeInclude,
			MatchIP: "0.0.0.0/0",
		})
	}
	return defaultConfig
}

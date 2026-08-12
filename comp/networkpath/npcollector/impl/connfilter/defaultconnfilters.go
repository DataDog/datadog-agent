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
		// Datadog intake endpoints (e.g. *.datadoghq.com) are CNAME'd to AWS
		// ELBs, so a connection's reverse-resolved domain is the ELB hostname
		// rather than the datadoghq.com name. The domain-based excludes above
		// miss those, causing dynamic tests to traceroute Datadog's own intake.
		// Exclude the ELB hostnames by their Datadog-owned load balancer name
		// prefix. Region-agnostic on purpose; the prefix is specific enough that
		// it won't match customer-owned ELBs.
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

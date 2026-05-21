// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connfilter

// getDefaultConnFilters returns the default connection filters.
//
// Datadog-owned domains and the configured site are excluded so that the
// collector does not trace its own infrastructure. The reserved internal-only
// TLDs `.local` (RFC 6762, mDNS) and `.internal` (IANA Special-Use Domain
// Names registry) are also excluded since traffic to those names is not a
// meaningful network path. Users can opt back in by adding an `include`
// filter under `network_path.collector.filters`; user filters are appended
// after the defaults and the last matching filter wins.
//
// More default filters are added for EUDM mode and can be found in
// `pkg/config/setup/config.go`.
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
			Type:                FilterTypeExclude,
			MatchDomain:         `.*\.local`,
			MatchDomainStrategy: MatchDomainStrategyRegex,
		},
		{
			Type:                FilterTypeExclude,
			MatchDomain:         `.*\.internal`,
			MatchDomainStrategy: MatchDomainStrategyRegex,
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

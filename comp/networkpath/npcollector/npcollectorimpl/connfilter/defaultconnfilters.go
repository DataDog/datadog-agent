// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connfilter

func getDefaultConnFilters(site string) []Config {
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
	}
	if site != "" {
		defaultConfig = append(defaultConfig, Config{
			Type:        FilterTypeExclude,
			MatchDomain: "*." + site,
		})
	}
	return defaultConfig
}

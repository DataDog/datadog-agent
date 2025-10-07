// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connfilter

func getDefaultConnFilters(site string) []Config {
	defaultConfig := []Config{
		{
			Type:        filterTypeExclude,
			MatchDomain: "*.datadog.pool.ntp.org",
		},
		{
			Type:        filterTypeExclude,
			MatchDomain: "*.datadoghq.com",
		},
		{
			Type:        filterTypeExclude,
			MatchDomain: "*." + site,
		},
	}
	return defaultConfig
}

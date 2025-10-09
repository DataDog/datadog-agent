// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package connfilter

import "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl/connfiltertype"

func getDefaultConnFilters(site string) []connfiltertype.Config {
	defaultConfig := []connfiltertype.Config{
		{
			Type:        connfiltertype.FilterTypeExclude,
			MatchDomain: "*.datadog.pool.ntp.org",
		},
		{
			Type:        connfiltertype.FilterTypeExclude,
			MatchDomain: "*.datadoghq.com",
		},
		{
			Type:        connfiltertype.FilterTypeExclude,
			MatchDomain: "*.datadoghq.eu",
		},
		{
			Type:        connfiltertype.FilterTypeExclude,
			MatchDomain: "*." + site,
		},
	}
	return defaultConfig
}

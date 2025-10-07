package connfilter

func getDefaultConnFilters(site string) []ConnFilterConfig {
	defaultConfig := []ConnFilterConfig{
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

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

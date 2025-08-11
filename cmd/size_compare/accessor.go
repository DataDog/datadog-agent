package main

var (
	globalConfig BuildableConfig
)

func GetConfig() Config {
	return globalConfig
}

func GetBuildableConfig() BuildableConfig {
	return globalConfig
}

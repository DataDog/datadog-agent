package main

var (
	globalConfig Config
)

func GetConfig() Config {
	return globalConfig
}

func GetBuildableConfig() BuildableConfig {
	return globalConfig.(BuildableConfig)
}

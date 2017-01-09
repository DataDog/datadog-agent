package common

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/loader"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// GetConfigProviders builds a list of providers for checks' configurations, the sequence defines
// the precedence.
func GetConfigProviders() (providers []loader.ConfigProvider) {
	confSearchPaths := []string{}
	for _, path := range configPaths {
		confSearchPaths = append(confSearchPaths, filepath.Join(path, "conf.d"))
	}

	// File Provider
	providers = append(providers, loader.NewFileConfigProvider(confSearchPaths))

	return providers
}

// GetCheckLoaders builds a list of check loaders, the sequence defines the precedence.
func GetCheckLoaders() []loader.CheckLoader {
	return []loader.CheckLoader{
		py.NewPythonCheckLoader(),
		core.NewGoCheckLoader(),
	}
}

// SetupConfig fires up the configuration system
func SetupConfig() {
	// set the paths where a config file is expected
	for _, path := range configPaths {
		config.Datadog.AddConfigPath(path)
	}

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	// define defaults for the Agent
	config.Datadog.SetDefault("cmd_sock", "/tmp/agent.sock")
	config.Datadog.BindEnv("cmd_sock")
}

// ReloadCheck should reload a check but expect the worst
func ReloadCheck(name string) {
	// unschedule
	AgentScheduler.Cancel(name)

	// reload configs

	// <shameless>
	// Get a list of config checks from the configured providers
	var configs []check.Config
	for _, provider := range GetConfigProviders() {
		c, _ := provider.Collect()
		for _, config := range c {
			if config.Name == name {
				configs = append(configs, config)
			}
		}
	}

	// given a list of configurations, try to load corresponding checks using different loaders
	loaders := GetCheckLoaders()
	for _, conf := range configs {
		for _, loader := range loaders {
			res, err := loader.Load(conf)
			if err == nil {
				for _, check := range res {
					AgentScheduler.Enter(check)
				}
			}
		}
	}
	// </shameless>
}

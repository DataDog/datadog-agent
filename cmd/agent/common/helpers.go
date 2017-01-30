package common

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	"github.com/DataDog/datadog-agent/pkg/collector/check/py"
	"github.com/DataDog/datadog-agent/pkg/collector/loader"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// GetConfigProviders builds a list of providers for checks' configurations, the sequence defines
// the precedence.
func GetConfigProviders(confdPath string) (providers []loader.ConfigProvider) {
	if confdPath == "" {
		confdPath = defaultConfdPath
	}

	confSearchPaths := []string{
		confdPath,
		filepath.Join(DistPath, "conf.d"),
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
	config.Datadog.AddConfigPath(defaultConfPath)
	config.Datadog.AddConfigPath(DistPath)

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}

	// define defaults for the Agent
	config.Datadog.SetDefault("cmd_sock", "/tmp/agent.sock")
	config.Datadog.BindEnv("cmd_sock")
}

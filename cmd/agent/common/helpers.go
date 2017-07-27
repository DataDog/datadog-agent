package common

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

// SetupAutoConfig instantiate the global AutoConfig object and sets up
// the Agent configuration providers and check loaders
func SetupAutoConfig(confdPath string) {
	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment
	coll := collector.NewCollector(GetDistPath(),
		PyChecksPath,
		filepath.Join(GetDistPath(), "checks"),
		config.Datadog.GetString("additional_checksd"))

	// create the Autoconfig instance
	AC = autodiscovery.NewAutoConfig(coll)

	// add the check loaders
	for _, loader := range loaders.LoaderCatalog() {
		AC.AddLoader(loader)
		log.Debugf("Added %s to AutoConfig", loader)
	}

	// add the configuration providers
	// File Provider
	confSearchPaths := []string{
		confdPath,
		filepath.Join(GetDistPath(), "conf.d"),
	}
	AC.AddProvider(providers.NewFileConfigProvider(confSearchPaths), false)

	// Register Providers
	for backend, provider := range providers.ProviderCatalog {
		AC.AddProvider(provider, true)
		log.Infof("Registering %s config provider", backend)
	}
}

// StartAutoConfig starts the autoconfig polling and loads the configs
func StartAutoConfig(confdPath string) {
	SetupAutoConfig(confdPath)
	AC.StartPolling()
	AC.LoadConfigs()
}

// SetupConfig fires up the configuration system
func SetupConfig(confFilePath string) {
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
	}
	config.Datadog.AddConfigPath(defaultConfPath)
	config.Datadog.AddConfigPath(GetDistPath())

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("unable to load Datadog config file: %s", err))
	}
}

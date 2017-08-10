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

// SetupAutoConfig configures the global AutoConfig:
//   1. add the configuration providers
//   2. add the check loaders
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

	// Add the configuration providers
	// File Provider is hardocded and always enabled
	confSearchPaths := []string{
		confdPath,
		filepath.Join(GetDistPath(), "conf.d"),
	}
	AC.AddProvider(providers.NewFileConfigProvider(confSearchPaths), false)

	// Register additional configuration providers
	for backend, provider := range providers.ProviderCatalog {
		AC.AddProvider(provider, true)
		log.Infof("Registering %s config provider", backend)
	}

	// add the service listeners
	// newService := make(chan listeners.Service)
	// delService := make(chan listeners.Service)

	// Docker listener
	// docker, err := listeners.NewDockerListener(newService, delService)
	// if err != nil {
	// 	log.Errorf("Failed to create a Docker listener. Is Docker accessible by the agent? %s", err)
	// } else {
	// 	AC.AddListener(docker)
	// }

	// add the config resolver
	// resolver := autodiscovery.NewConfigResolver(newService, delService)
	// AC.RegisterConfigResolver(resolver)
}

// StartAutoConfig starts the autoconfig:
//   1. start polling the providers
//   2. load all the configurations available at startup
//   3. run all the Checks for each configuration found
func StartAutoConfig() {
	AC.StartPolling()
	AC.LoadAndRun()
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

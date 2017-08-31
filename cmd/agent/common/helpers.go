// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

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
	var CP []config.ConfigurationProviders
	err := config.Datadog.UnmarshalKey("config_providers", &CP)
	if err == nil {
		for _, cp := range CP {
			_, found := providers.ProviderCatalog[cp.Name]
			if found {
				var err = fmt.Errorf("provider %v is not supported", cp.Name)
				var configProvider providers.ConfigProvider
				switch cp.Name {
				case "etcd":
					configProvider, err = providers.NewEtcdConfigProvider(cp)
				case "consul":
					configProvider, err = providers.NewConsulConfigProvider(cp)
				case "zookeeper":
					configProvider, err = providers.NewZookeeperConfigProvider(cp)
				}

				if err == nil {
					AC.AddProvider(configProvider, cp.Polling)
					log.Infof("Registering %s config provider", cp.Name)
				} else {
					log.Errorf("Error while adding config provider %v: %v", cp.Name, err)
				}
			}
		}
	}

	// Docker listener
	// docker, err := listeners.NewDockerListener(newService, delService)
	// if err != nil {
	// 	log.Errorf("Failed to create a Docker listener. Is Docker accessible by the agent? %s", err)
	// } else {
	// 	AC.AddListener(docker)
	// }
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

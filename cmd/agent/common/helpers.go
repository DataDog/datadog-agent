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
	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
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
	coll := collector.NewCollector(GetPythonPaths()...)

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
			factory, found := providers.ProviderCatalog[cp.Name]
			if found {
				configProvider, err := factory(cp)
				if err == nil {
					AC.AddProvider(configProvider, cp.Polling)
					log.Infof("Registering %s config provider", cp.Name)
				} else {
					log.Errorf("Error while adding config provider %v: %v", cp.Name, err)
				}
			} else {
				log.Errorf("Unable to find this provider in the catalog: %v", cp.Name)
			}
		}
	} else {
		log.Errorf("Error while reading 'config_providers' settings: %v", err)
	}

	// Autodiscovery listeners
	// for now, no need to implement a registry of available listeners since we
	// have only docker
	var Listeners []config.Listeners
	if err = config.Datadog.UnmarshalKey("listeners", &Listeners); err == nil {
		for _, l := range Listeners {
			if l.Name == "docker" {
				docker, err := listeners.NewDockerListener()
				if err != nil {
					log.Errorf("Failed to create a Docker listener. Is Docker accessible by the agent? %s", err)
				} else {
					AC.AddListener(docker)
				}
				break
			}
		}
	}
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
func SetupConfig(confFilePath string) error {
	// set the paths where a config file is expected
	if len(confFilePath) != 0 {
		// if the configuration file path was supplied on the command line,
		// add that first so it's first in line
		config.Datadog.AddConfigPath(confFilePath)
	}
	config.Datadog.AddConfigPath(DefaultConfPath)
	config.Datadog.AddConfigPath(GetDistPath())

	// load the configuration
	err := config.Datadog.ReadInConfig()
	if err != nil {
		return fmt.Errorf("unable to load Datadog config file: %s", err)
	}
	return nil
}

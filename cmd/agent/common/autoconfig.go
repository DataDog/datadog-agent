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
	"github.com/DataDog/datadog-agent/pkg/tagger"
	log "github.com/cihub/seelog"
)

// SetupAutoConfig configures the global AutoConfig:
//   1. add the configuration providers
//   2. add the check loaders
func SetupAutoConfig(confdPath string) {
	// start tagging system
	err := tagger.Init()
	if err != nil {
		fmt.Printf("Unable to start tagging system: %s", err)
	}

	// create the Collector instance and start all the components
	// NOTICE: this will also setup the Python environment, if available
	Coll = collector.NewCollector(GetPythonPaths()...)

	// create the Autoconfig instance
	AC = autodiscovery.NewAutoConfig(Coll)

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
	err = config.Datadog.UnmarshalKey("config_providers", &CP)
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
			serviceListenerFactory, ok := listeners.ServiceListenerFactories[l.Name]
			if !ok {
				// Factory has not been registered.
				log.Warnf("Listener %s was not registered", l)
				continue
			}
			serviceListener, err := serviceListenerFactory()
			if err != nil {
				log.Errorf("Failed to create a %s listener: %s", l.Name, err)
			} else {
				AC.AddListener(serviceListener)
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

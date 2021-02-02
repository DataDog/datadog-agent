// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	confad "github.com/DataDog/datadog-agent/pkg/config/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func setupAutoDiscovery(confSearchPaths []string, metaScheduler *scheduler.MetaScheduler) *autodiscovery.AutoConfig {
	ad := autodiscovery.NewAutoConfig(metaScheduler)
	ad.AddConfigProvider(providers.NewFileConfigProvider(confSearchPaths), false, 0)

	// Autodiscovery cannot easily use config.RegisterOverrideFunc() due to Unmarshalling
	var discoveredProviders []config.ConfigurationProviders
	var discoveredListeners []config.Listeners
	if config.Datadog.GetBool("autoconf_from_environment") {
		discoveredProviders, discoveredListeners = confad.DiscoverComponentsFromEnv()
	}

	providersFromConfig, listenersFromConfig := confad.DiscoverComponentsFromConfig()
	discoveredProviders = append(discoveredProviders, providersFromConfig...)
	discoveredListeners = append(discoveredListeners, listenersFromConfig...)

	// Register additional configuration providers
	var configProviders []config.ConfigurationProviders
	var uniqueConfigProviders map[string]config.ConfigurationProviders
	err := config.Datadog.UnmarshalKey("config_providers", &configProviders)

	if err == nil {
		uniqueConfigProviders = make(map[string]config.ConfigurationProviders, len(configProviders)+len(discoveredProviders))
		for _, provider := range configProviders {
			uniqueConfigProviders[provider.Name] = provider
		}

		// Add extra config providers
		for _, name := range config.Datadog.GetStringSlice("extra_config_providers") {
			if _, found := uniqueConfigProviders[name]; !found {
				uniqueConfigProviders[name] = config.ConfigurationProviders{Name: name, Polling: true}
			} else {
				log.Infof("Duplicate AD provider from extra_config_providers discarded as already present in config_providers: %s", name)
			}
		}

		for _, provider := range discoveredProviders {
			if _, found := uniqueConfigProviders[provider.Name]; !found {
				uniqueConfigProviders[provider.Name] = provider
			}
		}

	} else {
		log.Errorf("Error while reading 'config_providers' settings: %v", err)
	}

	// Adding all found providers
	for _, cp := range uniqueConfigProviders {
		factory, found := providers.ProviderCatalog[cp.Name]
		if found {
			configProvider, err := factory(cp)
			if err != nil {
				log.Errorf("Error while adding config provider %v: %v", cp.Name, err)
				continue
			}

			pollInterval := providers.GetPollInterval(cp)
			if cp.Polling {
				log.Infof("Registering %s config provider polled every %s", cp.Name, pollInterval.String())
			} else {
				log.Infof("Registering %s config provider", cp.Name)
			}
			ad.AddConfigProvider(configProvider, cp.Polling, pollInterval)
		} else {
			log.Errorf("Unable to find this provider in the catalog: %v", cp.Name)
		}
	}

	var listeners []config.Listeners
	err = config.Datadog.UnmarshalKey("listeners", &listeners)
	if err == nil {
		// Add extra listeners
		for _, name := range config.Datadog.GetStringSlice("extra_listeners") {
			listeners = append(listeners, config.Listeners{Name: name})
		}

		for _, listener := range discoveredListeners {
			alreadyPresent := false
			for _, existingListener := range listeners {
				if listener.Name == existingListener.Name {
					alreadyPresent = true
					break
				}
			}

			if !alreadyPresent {
				listeners = append(listeners, listener)
			}
		}

		ad.AddListeners(listeners)
	} else {
		log.Errorf("Error while reading 'listeners' settings: %v", err)
	}

	return ad
}

// StartAutoConfig starts auto discovery
func StartAutoConfig() {
	AC.LoadAndRun()
}

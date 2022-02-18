// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	confad "github.com/DataDog/datadog-agent/pkg/config/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This is due to an AD limitation that does not allow several listeners to work in parallel
// if they can provide for the same objects.
// When this is solved, we can remove this check and simplify code below
var (
	incompatibleListeners = map[string]map[string]struct{}{
		"kubelet":   {"container": struct{}{}},
		"container": {"kubelet": struct{}{}},
	}
)

func setupAutoDiscovery(confSearchPaths []string, metaScheduler *scheduler.MetaScheduler) *autodiscovery.AutoConfig {
	ad := autodiscovery.NewAutoConfig(metaScheduler)
	providers.InitConfigFilesReader(confSearchPaths)
	ad.AddConfigProvider(providers.NewFileConfigProvider(), false, 0)

	// Autodiscovery cannot easily use config.RegisterOverrideFunc() due to Unmarshalling
	extraConfigProviders, extraConfigListeners := confad.DiscoverComponentsFromConfig()

	var extraEnvProviders []config.ConfigurationProviders
	var extraEnvListeners []config.Listeners
	if config.IsAutoconfigEnabled() && !config.IsCLCRunner() {
		extraEnvProviders, extraEnvListeners = confad.DiscoverComponentsFromEnv()
	}

	// Register additional configuration providers
	var configProviders []config.ConfigurationProviders
	var uniqueConfigProviders map[string]config.ConfigurationProviders
	err := config.Datadog.UnmarshalKey("config_providers", &configProviders)

	if err == nil {
		uniqueConfigProviders = make(map[string]config.ConfigurationProviders, len(configProviders)+len(extraEnvProviders)+len(configProviders))
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

		// The "docker" config provider was replaced with the "container" one
		// that supports Docker, but also other runtimes. We need this
		// conversion to avoid breaking configs that included "docker".
		if options, found := uniqueConfigProviders["docker"]; found {
			delete(uniqueConfigProviders, "docker")
			options.Name = names.Container
			uniqueConfigProviders["container"] = options
		}

		for _, provider := range extraConfigProviders {
			if _, found := uniqueConfigProviders[provider.Name]; !found {
				uniqueConfigProviders[provider.Name] = provider
			}
		}

		for _, provider := range extraEnvProviders {
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
			configProvider, err := factory(&cp)
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

		// The "docker" listener was replaced with the "container" one that
		// supports Docker, but also other runtimes. We need this conversion to
		// avoid breaking configs that included "docker".
		for i := range listeners {
			if listeners[i].Name == "docker" {
				listeners[i].Name = "container"
			}
		}

		for _, listener := range extraConfigListeners {
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

		// For extraEnvListeners, we need to check incompatibleListeners to avoid generation of duplicate checks
		for _, listener := range extraEnvListeners {
			skipListener := false
			incomp := incompatibleListeners[listener.Name]

			for _, existingListener := range listeners {
				if listener.Name == existingListener.Name {
					skipListener = true
					break
				}

				if _, found := incomp[existingListener.Name]; found {
					log.Debugf("Discarding discovered listener: %s as incompatible with listener from config: %s", listener.Name, existingListener.Name)
					skipListener = true
					break
				}
			}

			if !skipListener {
				listeners = append(listeners, listener)
			}
		}

		// Fill listeners settings
		providersSet := make(map[string]struct{}, len(uniqueConfigProviders))
		for provider := range uniqueConfigProviders {
			providersSet[provider] = struct{}{}
		}

		for i := range listeners {
			listeners[i].SetEnabledProviders(providersSet)
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

// WaitForConfigs retries the collection of Autodiscovery configs until the checkMatcher function (which
// defines whether the list of integration configs collected is sufficient) returns true or the timeout is reached.
// Autodiscovery listeners run asynchronously, AC.GetAllConfigs() can fail at the beginning to resolve templated configs
// depending on non-deterministic factors (system load, network latency, active Autodiscovery listeners and their configurations).
// This function improves the resiliency of the check command.
// Note: If the check corresponds to a non-template configuration it should be found on the first try and fast-returned.
func WaitForConfigs(retryInterval, timeout time.Duration, checkMatcher func([]integration.Config) bool) []integration.Config {
	allConfigs := AC.GetAllConfigs()
	if checkMatcher(allConfigs) {
		return allConfigs
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	retryTicker := time.NewTicker(retryInterval)
	defer retryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return allConfigs
		case <-retryTicker.C:
			allConfigs = AC.GetAllConfigs()
			if checkMatcher(allConfigs) {
				return allConfigs
			}
		}
	}
}

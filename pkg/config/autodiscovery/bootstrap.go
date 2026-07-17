// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"path/filepath"
	"time"

	autodiscoverydef "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
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

	legacyProviders = []string{"kubelet", "container", "docker"}
)

// LoadComponents configures several common Agent components:
// tagger, collector, scheduler and autodiscovery
func LoadComponents(ac autodiscoverydef.Component, cfg config.Component) {
	confdPath := cfg.GetString("confd_path")

	confSearchPaths := []string{
		confdPath,
		filepath.Join(defaultpaths.GetDistPath(), "conf.d"),
		"",
	}

	setupAutoDiscovery(confSearchPaths, ac, cfg)
}

func setupAutoDiscovery(confSearchPaths []string, ac autodiscoverydef.Component, cfg config.Component) {
	if cfg.GetString("fleet_policies_dir") != "" {
		confSearchPaths = append(confSearchPaths, filepath.Join(cfg.GetString("fleet_policies_dir"), "conf.d"))
	}

	providers.InitConfigFilesReader(confSearchPaths)

	acTelemetryStore := ac.GetTelemetryStore()

	ac.AddConfigProvider(
		providers.NewFileConfigProvider(acTelemetryStore),
		cfg.GetBool("autoconf_config_files_poll"),
		time.Duration(cfg.GetInt("autoconf_config_files_poll_interval"))*time.Second,
	)

	// Autodiscovery cannot easily use config.RegisterOverrideFunc() due to Unmarshalling
	extraConfigProviders, extraConfigListeners := DiscoverComponentsFromConfig(cfg)

	var extraEnvProviders []pkgconfigsetup.ConfigurationProviders
	var extraEnvListeners []pkgconfigsetup.Listeners
	if pkgconfigenv.IsAutoconfigEnabled(cfg) && !pkgconfigsetup.IsCLCRunner(cfg) {
		extraEnvProviders, extraEnvListeners = DiscoverComponentsFromEnv(cfg)
	}

	// Register additional configuration providers
	var configProviders []pkgconfigsetup.ConfigurationProviders
	var uniqueConfigProviders map[string]pkgconfigsetup.ConfigurationProviders
	err := structure.UnmarshalKey(cfg, "config_providers", &configProviders)

	if err == nil {
		uniqueConfigProviders = make(map[string]pkgconfigsetup.ConfigurationProviders, len(configProviders)+len(extraEnvProviders)+len(configProviders))
		for _, provider := range configProviders {
			uniqueConfigProviders[provider.Name] = provider
		}

		// Add extra config providers
		for _, name := range cfg.GetStringSlice("extra_config_providers") {
			if _, found := uniqueConfigProviders[name]; !found {
				uniqueConfigProviders[name] = pkgconfigsetup.ConfigurationProviders{Name: name, Polling: true}
			} else {
				log.Infof("Duplicate AD provider from extra_config_providers discarded as already present in config_providers: %s", name)
			}
		}

		var enableContainerProvider bool
		for _, p := range legacyProviders {
			if _, found := uniqueConfigProviders[p]; found {
				enableContainerProvider = true
				delete(uniqueConfigProviders, p)
			}
		}

		if enableContainerProvider {
			uniqueConfigProviders[names.KubeContainer] = pkgconfigsetup.ConfigurationProviders{Name: names.KubeContainer}
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
		if err := ac.AddConfigProviderFromCatalog(cp); err != nil {
			log.Errorf("%v", err)
		}
	}

	var listeners []pkgconfigsetup.Listeners
	err = structure.UnmarshalKey(cfg, "listeners", &listeners)
	if err == nil {
		// Add extra listeners
		for _, name := range cfg.GetStringSlice("extra_listeners") {
			listeners = append(listeners, pkgconfigsetup.Listeners{Name: name})
		}

		// The "docker" and "ecs" listeners were replaced with the
		// "container" one that supports several container runtimes. We
		// need this conversion to avoid breaking older configs that
		// included the older listeners.
		for i := range listeners {
			if listeners[i].Name == "docker" || listeners[i].Name == "ecs" {
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

		ac.AddListeners(listeners)
	} else {
		log.Errorf("Error while reading 'listeners' settings: %v", err)
	}
}

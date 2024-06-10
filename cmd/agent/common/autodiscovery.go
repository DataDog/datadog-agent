// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/atomic"
	utilserror "k8s.io/apimachinery/pkg/util/errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	confad "github.com/DataDog/datadog-agent/pkg/config/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
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

func setupAutoDiscovery(confSearchPaths []string, wmeta workloadmeta.Component, ac autodiscovery.Component) {
	providers.InitConfigFilesReader(confSearchPaths)
	ac.AddConfigProvider(
		providers.NewFileConfigProvider(),
		config.Datadog().GetBool("autoconf_config_files_poll"),
		time.Duration(config.Datadog().GetInt("autoconf_config_files_poll_interval"))*time.Second,
	)

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
	err := config.Datadog().UnmarshalKey("config_providers", &configProviders)

	if err == nil {
		uniqueConfigProviders = make(map[string]config.ConfigurationProviders, len(configProviders)+len(extraEnvProviders)+len(configProviders))
		for _, provider := range configProviders {
			uniqueConfigProviders[provider.Name] = provider
		}

		// Add extra config providers
		for _, name := range config.Datadog().GetStringSlice("extra_config_providers") {
			if _, found := uniqueConfigProviders[name]; !found {
				uniqueConfigProviders[name] = config.ConfigurationProviders{Name: name, Polling: true}
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
			uniqueConfigProviders[names.KubeContainer] = config.ConfigurationProviders{Name: names.KubeContainer}
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
		factory, found := ac.GetProviderCatalog()[cp.Name]
		if found {
			configProvider, err := factory(&cp, wmeta)
			if err != nil {
				log.Errorf("Error while adding config provider %v: %v", cp.Name, err)
				continue
			}

			pollInterval := providers.GetPollInterval(cp)
			ac.AddConfigProvider(configProvider, cp.Polling, pollInterval)
		} else {
			log.Errorf("Unable to find this provider in the catalog: %v", cp.Name)
		}
	}

	var listeners []config.Listeners
	err = config.Datadog().UnmarshalKey("listeners", &listeners)
	if err == nil {
		// Add extra listeners
		for _, name := range config.Datadog().GetStringSlice("extra_listeners") {
			listeners = append(listeners, config.Listeners{Name: name})
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

// schedulerFunc is a type alias to allow a function to be used as an AD scheduler
type schedulerFunc func([]integration.Config)

// Schedule implements scheduler.Scheduler#Schedule.
func (sf schedulerFunc) Schedule(configs []integration.Config) {
	sf(configs)
}

// Unschedule implements scheduler.Scheduler#Unschedule.
func (sf schedulerFunc) Unschedule(_ []integration.Config) {
	// (do nothing)
}

// Stop implements scheduler.Scheduler#Stop.
func (sf schedulerFunc) Stop() {
}

// WaitForConfigsFromAD waits until a count of discoveryMinInstances configs
// with names in checkNames are scheduled by AD, and returns the matches.
//
// If the context is cancelled, then any accumulated, matching changes are
// returned, even if that is fewer than discoveryMinInstances.
func WaitForConfigsFromAD(ctx context.Context,
	checkNames []string,
	discoveryMinInstances int,
	instanceFilter string,
	ac autodiscovery.Component) (configs []integration.Config, lastError error) {
	return waitForConfigsFromAD(ctx, false, checkNames, discoveryMinInstances, instanceFilter, ac)
}

// WaitForAllConfigsFromAD waits until its context expires, and then returns
// the full set of checks scheduled by AD.
func WaitForAllConfigsFromAD(ctx context.Context, ac autodiscovery.Component) (configs []integration.Config, lastError error) {
	return waitForConfigsFromAD(ctx, true, []string{}, 0, "", ac)
}

// waitForConfigsFromAD waits for configs from the AD scheduler and returns them.
//
// AD scheduling is asynchronous, so this is a time-based process.
//
// If wildcard is false, this waits until at least discoveryMinInstances
// configs with names in checkNames are scheduled by AD, and returns the
// matches.  If the context is cancelled before that occurs, then any
// accumulated configs are returned, even if that is fewer than
// discoveryMinInstances.
//
// If wildcard is true, this gathers all configs scheduled before the context
// is cancelled, and then returns.  It will not return before the context is
// cancelled.
func waitForConfigsFromAD(ctx context.Context,
	wildcard bool,
	checkNames []string,
	discoveryMinInstances int,
	instanceFilter string,
	ac autodiscovery.Component) (configs []integration.Config, returnErr error) {
	configChan := make(chan integration.Config)

	// signal to the scheduler when we are no longer waiting, so we do not continue
	// to push items to configChan
	waiting := atomic.NewBool(true)
	defer func() {
		waiting.Store(false)
		// ..and drain any message currently pending in the channel
		select {
		case <-configChan:
		default:
		}
	}()

	var match func(cfg integration.Config) bool
	if wildcard {
		// match all configs
		match = func(integration.Config) bool { return true }
	} else {
		// match configs with names in checkNames
		match = func(cfg integration.Config) bool {
			for _, checkName := range checkNames {
				if cfg.Name == checkName {
					return true
				}
			}
			return false
		}
	}

	stopChan := make(chan struct{})
	// add the scheduler in a goroutine, since it will schedule any "catch-up" immediately,
	// placing items in configChan
	go ac.AddScheduler(adtypes.CheckCmdName, schedulerFunc(func(configs []integration.Config) {
		var errorList []error
		for _, cfg := range configs {
			if instanceFilter != "" {
				instances, filterErrors := filterInstances(cfg.Instances, instanceFilter)
				if len(filterErrors) > 0 {
					errorList = append(errorList, filterErrors...)
					continue
				}
				if len(instances) == 0 {
					continue
				}
				cfg.Instances = instances
			}

			if match(cfg) && waiting.Load() {
				configChan <- cfg
			}
		}
		if len(errorList) > 0 {
			returnErr = errors.New(utilserror.NewAggregate(errorList).Error())
			stopChan <- struct{}{}
		}
	}), true)

	for wildcard || len(configs) < discoveryMinInstances {
		select {
		case cfg := <-configChan:
			configs = append(configs, cfg)
		case <-stopChan:
			return
		case <-ctx.Done():
			return
		}
	}
	return
}

func filterInstances(instances []integration.Data, instanceFilter string) ([]integration.Data, []error) {
	var newInstances []integration.Data
	var errors []error
	for _, instance := range instances {
		exist, err := jsonquery.YAMLCheckExist(instance, instanceFilter)
		if err != nil {
			errors = append(errors, fmt.Errorf("instance filter error: %v", err))
			continue
		}
		if exist {
			newInstances = append(newInstances, instance)
		}
	}
	return newInstances, errors
}

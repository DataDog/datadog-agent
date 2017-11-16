// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package autodiscovery

import (
	"expvar"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/listeners"
	"github.com/DataDog/datadog-agent/pkg/collector/providers"
	log "github.com/cihub/seelog"
)

var (
	configsPollIntl = 10 * time.Second
	configPipeBuf   = 100
	acErrors        *expvar.Map
	errorStats      = newAcErrorStats()
)

func init() {
	acErrors = expvar.NewMap("autoconfig")
	acErrors.Set("ConfigErrors", expvar.Func(func() interface{} {
		return errorStats.getConfigErrors()
	}))
	acErrors.Set("LoaderErrors", expvar.Func(func() interface{} {
		return errorStats.getLoaderErrors()
	}))
	acErrors.Set("RunErrors", expvar.Func(func() interface{} {
		return errorStats.getRunErrors()
	}))
}

// providerDescriptor keeps track of the configurations loaded by a certain
// `providers.ConfigProvider` and whether it should be polled or not.
type providerDescriptor struct {
	provider providers.ConfigProvider
	configs  []check.Config
	poll     bool
}

// AutoConfig is responsible to collect checks configurations from
// different sources and then create, update or destroy check instances.
// It owns and orchestrates several key modules:
//  - it owns a reference to the `collector.Collector` that it uses to schedule checks when template or container updates warrant them
//  - it holds a list of `providers.ConfigProvider`s and poll them according to their policy
//  - it holds a list of `check.Loader`s to load configurations into `Check` objects
//  - it holds a list of `listeners.ServiceListener`s` used to listen to container lifecycle events
//  - it runs the `ConfigResolver` that resolves a configuration template to an actual configuration based on data it extracts from a service that matches it the template
//
// Notice the `AutoConfig` public API speaks in terms of `check.Config`,
// meaning that you cannot use it to schedule check instances directly.
type AutoConfig struct {
	collector         *collector.Collector
	providers         []*providerDescriptor
	loaders           []check.Loader
	templateCache     *TemplateCache
	listeners         []listeners.ServiceListener
	configResolver    *ConfigResolver
	configsPollTicker *time.Ticker
	config2checks     map[string][]check.ID       // cache the ID of checks we load for each config
	name2jmxmetrics   map[string]check.ConfigData // holds the metrics to collect for JMX checks
	stop              chan bool
	pollerActive      bool
	m                 sync.RWMutex
}

// NewAutoConfig creates an AutoConfig instance.
func NewAutoConfig(collector *collector.Collector) *AutoConfig {
	ac := &AutoConfig{
		collector:     collector,
		providers:     make([]*providerDescriptor, 0, 5),
		loaders:       make([]check.Loader, 0, 5),
		templateCache: NewTemplateCache(),
		config2checks: make(map[string][]check.ID),
		stop:          make(chan bool),
	}
	ac.configResolver = newConfigResolver(collector, ac, ac.templateCache)

	return ac
}

// StartPolling starts the goroutine responsible for polling the providers
func (ac *AutoConfig) StartPolling() {
	ac.m.Lock()
	defer ac.m.Unlock()

	ac.configsPollTicker = time.NewTicker(configsPollIntl)
	ac.pollConfigs()
	ac.pollerActive = true
}

// Stop just shuts down AutoConfig in a clean way.
// AutoConfig is not supposed to be restarted, so this is expected
// to be called only once at program exit.
func (ac *AutoConfig) Stop() {
	ac.m.Lock()
	defer ac.m.Unlock()

	// stop the poller if running
	if ac.pollerActive {
		ac.stop <- true
		ac.pollerActive = false
	}

	// stop the collector
	if ac.collector != nil {
		ac.collector.Stop()
	}

	// stop the config resolver
	if ac.configResolver != nil {
		ac.configResolver.Stop()
	}

	// stop all the listeners
	for _, l := range ac.listeners {
		l.Stop()
	}
}

// AddProvider adds a new configuration provider to AutoConfig.
// Callers must pass a flag to indicate whether the configuration provider
// expects to be polled or it's fine for it to be invoked only once in the
// Agent lifetime.
func (ac *AutoConfig) AddProvider(provider providers.ConfigProvider, shouldPoll bool) {
	ac.m.Lock()
	defer ac.m.Unlock()

	for _, pd := range ac.providers {
		if pd.provider == provider {
			// we already know this configuration provider, don't do anything

			// this is formatted inline since logging is done on a background thread,
			// so you can only pass it things to act on if they're thread safe
			// this is not inherently thread safe
			log.Warn(fmt.Sprintf("Provider %s was already added, skipping...", provider))
			return
		}
	}

	pd := &providerDescriptor{
		provider: provider,
		configs:  []check.Config{},
		poll:     shouldPoll,
	}
	ac.providers = append(ac.providers, pd)
}

// LoadAndRun loads all of the configs it can find and schedules the corresponding
// Check instances. Should always be run once so providers that don't need
// polling will be queried at least once
func (ac *AutoConfig) LoadAndRun() {
	resolvedConfigs := ac.getAllConfigs()
	checks := ac.getChecksFromConfigs(resolvedConfigs, true)
	ac.schedule(checks)
}

// GetChecksByName returns any Check instance we can load for the given
// check name
func (ac *AutoConfig) GetChecksByName(checkName string) []check.Check {
	// try to also match `FooCheck` if `foo` was passed
	titleCheck := fmt.Sprintf("%s%s", strings.Title(checkName), "Check")
	checks := []check.Check{}

	for _, check := range ac.getChecksFromConfigs(ac.getAllConfigs(), false) {
		if checkName == check.String() || titleCheck == check.String() {
			checks = append(checks, check)
		}
	}

	return checks
}

// getAllConfigs queries all the providers and returns all the check
// configurations found, resolving the ones it can
func (ac *AutoConfig) getAllConfigs() []check.Config {
	rawConfigs := []check.Config{}
	for _, pd := range ac.providers {
		cfgs, _ := pd.provider.Collect()

		if fileConfPd, ok := pd.provider.(*providers.FileConfigProvider); ok {
			// Grab any errors that occurred when reading the YAML file
			for name, e := range fileConfPd.Errors {
				errorStats.setConfigError(name, e)
			}

			for _, cfg := range cfgs {
				// JMX checks can have 2 YAML files: one containing the metrics to collect, one containing the
				// instance configuration
				// If the file provider finds any of these metric YAMLs, we store them in a map for future access
				if cfg.MetricConfig != nil {
					ac.name2jmxmetrics[cfg.Name] = cfg.MetricConfig
				}

				// Clear any old errors if a valid config file is found
				errorStats.removeConfigError(cfg.Name)
			}
		}
		rawConfigs = append(rawConfigs, cfgs...)
	}
	resolvedConfigs := []check.Config{}
	for _, config := range rawConfigs {
		rc, err := ac.resolve(config)
		if err != nil {
			log.Error(err)
		}
		resolvedConfigs = append(resolvedConfigs, rc...)
	}

	return resolvedConfigs
}

// getChecksFromConfigs gets all the check instances for given configurations
// optionally can populate ac cache config2checks
func (ac *AutoConfig) getChecksFromConfigs(configs []check.Config, populateCache bool) []check.Check {
	allChecks := []check.Check{}
	for _, config := range configs {
		configDigest := config.Digest()
		checks, err := ac.GetChecks(config)
		if err != nil {
			continue
		}
		for _, check := range checks {
			allChecks = append(allChecks, check)
			if populateCache {
				// store the checks we schedule for this config locally
				ac.config2checks[configDigest] = append(ac.config2checks[configDigest], check.ID())
			}
		}
	}

	return allChecks
}

// schedule takes a slice of checks and schedule them
func (ac *AutoConfig) schedule(checks []check.Check) {
	for _, check := range checks {
		log.Infof("Scheduling check %s", check)
		_, err := ac.collector.RunCheck(check)
		if err != nil {
			log.Errorf("Unable to run Check %s: %v", check, err)
			errorStats.setRunError(check.ID(), err.Error())
			continue
		}
	}
}

// resolve loads and resolves a given config and can optionnaly schedules the
// corresponding Check instances. Returns a slice of resolved configs
func (ac *AutoConfig) resolve(config check.Config) ([]check.Config, error) {
	configs := []check.Config{}

	// add default metrics to collect to JMX checks
	if config.CollectDefaultMetrics() {
		metrics, ok := ac.name2jmxmetrics[config.Name]
		if !ok {
			log.Infof("%s doesn't have an additional metric configuration file: not collecting default metrics", config.Name)
		} else if err := config.AddMetrics(metrics); err != nil {
			log.Infof("Unable to add default metrics to collect to %s check: %s", config.Name, err)
		}
	}

	if config.IsTemplate() {
		// store the template in the cache in any case
		if err := ac.templateCache.Set(config); err != nil {
			log.Errorf("Unable to store Check configuration in the cache: %s", err)
		}

		// try to resolve the template
		resolvedConfigs := ac.configResolver.ResolveTemplate(config)
		if len(resolvedConfigs) == 0 {
			log.Infof("Can't resolve the template for %s at this moment.", config.Name)
			return configs, nil
		}

		// If success, get the checks for each config resolved
		// and schedule for running, each template can resolve
		// to multiple configs
		for _, config := range resolvedConfigs {
			configs = append(configs, config)
		}
	} else {
		configs = append(configs, config)
	}

	return configs, nil
}

// AddListener adds a service listener to AutoConfig.
func (ac *AutoConfig) AddListener(listener listeners.ServiceListener) {
	ac.m.Lock()
	defer ac.m.Unlock()

	for _, l := range ac.listeners {
		if l == listener {
			log.Warnf("Listener %s was already added, skipping...", listener)
			return
		}
	}

	ac.listeners = append(ac.listeners, listener)
	listener.Listen(ac.configResolver.newService, ac.configResolver.delService)
}

// AddLoader adds a new Loader that AutoConfig can use to load a check.
func (ac *AutoConfig) AddLoader(loader check.Loader) {
	for _, l := range ac.loaders {
		if l == loader {
			log.Warnf("Loader %s was already added, skipping...", loader)
			return
		}
	}

	ac.loaders = append(ac.loaders, loader)
}

// pollConfigs periodically calls Collect() on all the configuration
// providers that have been requested to be polled
func (ac *AutoConfig) pollConfigs() {
	go func() {
		for {
			select {
			case <-ac.stop:
				if ac.configsPollTicker != nil {
					ac.configsPollTicker.Stop()
				}
				return
			case <-ac.configsPollTicker.C:
				ac.m.RLock()
				// invoke Collect on the known providers
				for _, pd := range ac.providers {
					// skip providers that don't want to be polled
					if !pd.poll {
						continue
					}

					// retrieve the list of newly added configurations as well
					// as removed configurations
					newConfigs, removedConfigs := ac.collect(pd)
					for _, config := range newConfigs {
						// store the checks we schedule for this config locally
						resolvedConfigs, err := ac.resolve(config)
						if err != nil {
							log.Error(err)
						}
						checks := ac.getChecksFromConfigs(resolvedConfigs, true)
						ac.schedule(checks)
					}

					for _, config := range removedConfigs {
						// unschedule all the checks corresponding to this config
						digest := config.Digest()
						ids := ac.config2checks[digest]
						stopped := map[check.ID]struct{}{}
						for _, id := range ids {
							// `StopCheck` might time out so we don't risk to block
							// the polling loop forever
							err := ac.collector.StopCheck(id)
							if err != nil {
								log.Errorf("Error stopping check %s: %s", id, err)
								errorStats.setRunError(id, err.Error())
							} else {
								stopped[id] = struct{}{}
							}
						}

						// remove the entry from `config2checks`
						if len(stopped) == len(ac.config2checks[digest]) {
							// we managed to stop all the checks for this config
							delete(ac.config2checks, digest)
						} else {
							// keep the checks we failed to stop in `config2checks`
							dangling := []check.ID{}
							for _, id := range ac.config2checks[digest] {
								if _, found := stopped[id]; !found {
									dangling = append(dangling, id)
								}
							}
							ac.config2checks[digest] = dangling
						}

						// if the config is a template, remove it from the cache
						if config.IsTemplate() {
							ac.templateCache.Del(config)
						}
					}
				}
				ac.m.RUnlock()
			}
		}
	}()
}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (ac *AutoConfig) collect(pd *providerDescriptor) (new, removed []check.Config) {
	new = []check.Config{}
	removed = []check.Config{}

	fetched, err := pd.provider.Collect()
	if err != nil {
		log.Errorf("Unable to collect configurations from provider %s: %s", pd.provider, err)
		return
	}

	for _, c := range fetched {
		if !pd.contains(&c) {
			new = append(new, c)
		}
	}

	old := pd.configs
	pd.configs = fetched

	for _, c := range old {
		if !pd.contains(&c) {
			removed = append(removed, c)
		}
	}

	log.Infof("%v: collected %d new configurations, removed %d", pd.provider, len(new), len(removed))

	return
}

// GetChecks takes a check configuration and returns a slice of Check instances
// along with any error it might happen during the process
func (ac *AutoConfig) GetChecks(config check.Config) ([]check.Check, error) {
	for _, loader := range ac.loaders {
		res, err := loader.Load(config)
		if err == nil {
			log.Infof("%v: successfully loaded check '%s'", loader, config.Name)
			errorStats.removeLoaderErrors(config.Name)
			return res, nil
		}

		errorStats.setLoaderError(config.Name, fmt.Sprintf("%v", loader), err.Error())

		// Check if some check instances were loaded correctly (can occur if there's multiple check instances)
		if len(res) != 0 {
			return res, nil
		}
		log.Debugf("%v: unable to load the check '%s': %s", loader, config.Name, err)
	}

	return []check.Check{}, fmt.Errorf("unable to load any check from config '%s'", config.Name)
}

// check if the descriptor contains the Config passed
func (pd *providerDescriptor) contains(c *check.Config) bool {
	for _, config := range pd.configs {
		if config.Equal(c) {
			return true
		}
	}

	return false
}

// GetLoaderErrors gets the errors from the loaderErrors struct
func GetLoaderErrors() map[string]LoaderErrors {
	return errorStats.getLoaderErrors()
}

// GetConfigErrors gets the config errors
func GetConfigErrors() map[string]string {
	return errorStats.getConfigErrors()
}

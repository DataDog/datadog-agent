// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"expvar"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
)

var (
	configsPollIntl = 10 * time.Second
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
	acErrors.Set("ResolveWarnings", expvar.Func(func() interface{} {
		return errorStats.getResolveWarnings()
	}))
}

// providerDescriptor keeps track of the configurations loaded by a certain
// `providers.ConfigProvider` and whether it should be polled or not.
type providerDescriptor struct {
	provider providers.ConfigProvider
	configs  []integration.Config
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
// Notice the `AutoConfig` public API speaks in terms of `integration.Config`,
// meaning that you cannot use it to schedule check instances directly.
type AutoConfig struct {
	collector         *collector.Collector
	providers         []*providerDescriptor
	loaders           []check.Loader
	templateCache     *TemplateCache
	listeners         []listeners.ServiceListener
	configResolver    *ConfigResolver
	configsPollTicker *time.Ticker
	configToChecks    map[string][]check.ID         // cache the ID of checks we load for each config
	checkToConfig     map[check.ID]string           // cache the config digest corresponding to a check
	nameToJMXMetrics  map[string]integration.Data   // holds the metrics to collect for JMX checks
	loadedConfigs     map[string]integration.Config // holds the resolved configs
	stop              chan bool
	pollerActive      bool
	health            *health.Handle
	store             *store
	m                 sync.RWMutex
}

// NewAutoConfig creates an AutoConfig instance.
func NewAutoConfig(collector *collector.Collector) *AutoConfig {
	ac := &AutoConfig{
		collector:        collector,
		providers:        make([]*providerDescriptor, 0, 5),
		loaders:          make([]check.Loader, 0, 5),
		templateCache:    NewTemplateCache(),
		configToChecks:   make(map[string][]check.ID),
		checkToConfig:    make(map[check.ID]string),
		nameToJMXMetrics: make(map[string]integration.Data),
		loadedConfigs:    make(map[string]integration.Config),
		stop:             make(chan bool),
		store:            newStore(),
		health:           health.Register("ad-autoconfig"),
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
			log.Warnf("Provider %s was already added, skipping...", provider)
			return
		}
	}

	pd := &providerDescriptor{
		provider: provider,
		configs:  []integration.Config{},
		poll:     shouldPoll,
	}
	ac.providers = append(ac.providers, pd)
}

// LoadAndRun loads all of the configs it can find and schedules the corresponding
// Check instances. Should always be run once so providers that don't need
// polling will be queried at least once
func (ac *AutoConfig) LoadAndRun() {
	resolvedConfigs := ac.GetAllConfigs()
	checks := ac.getChecksFromConfigs(resolvedConfigs, true)
	ac.schedule(checks)
}

// GetChecksByName returns any Check instance we can load for the given
// check name
func (ac *AutoConfig) GetChecksByName(checkName string) []check.Check {
	// try to also match `FooCheck` if `foo` was passed
	titleCheck := fmt.Sprintf("%s%s", strings.Title(checkName), "Check")
	var checks []check.Check

	for _, c := range ac.getChecksFromConfigs(ac.GetAllConfigs(), false) {
		if checkName == c.String() || titleCheck == c.String() {
			checks = append(checks, c)
		}
	}

	return checks
}

// GetAllConfigs queries all the providers and returns all the integration
// configurations found, resolving the ones it can
func (ac *AutoConfig) GetAllConfigs() []integration.Config {
	var resolvedConfigs []integration.Config

	ac.m.Lock()
	defer ac.m.Unlock()
	for _, pd := range ac.providers {
		cfgs, err := pd.provider.Collect()
		if err != nil {
			log.Debugf("Unexpected error returned when collecting provider %s: %v", pd.provider.String(), err)
		}

		if fileConfPd, ok := pd.provider.(*providers.FileConfigProvider); ok {
			var goodConfs []integration.Config
			for _, cfg := range cfgs {
				// JMX checks can have 2 YAML files: one containing the metrics to collect, one containing the
				// instance configuration
				// If the file provider finds any of these metric YAMLs, we store them in a map for future access
				if cfg.MetricConfig != nil {
					ac.nameToJMXMetrics[cfg.Name] = cfg.MetricConfig
					// We don't want to save metric files, it's enough to store them in the map
					continue
				}

				goodConfs = append(goodConfs, cfg)

				// Clear any old errors if a valid config file is found
				errorStats.removeConfigError(cfg.Name)
			}

			// Grab any errors that occurred when reading the YAML file
			for name, e := range fileConfPd.Errors {
				errorStats.setConfigError(name, e)
			}

			cfgs = goodConfs
		}
		// Store all raw configs in the provider
		pd.configs = cfgs

		// resolve configs if needed
		for _, config := range cfgs {
			config.Provider = pd.provider.String()
			rc := ac.resolve(config)
			resolvedConfigs = append(resolvedConfigs, rc...)
		}
	}

	return resolvedConfigs
}

// getChecksFromConfigs gets all the check instances for given configurations
// optionally can populate ac cache configToChecks
func (ac *AutoConfig) getChecksFromConfigs(configs []integration.Config, populateCache bool) []check.Check {
	var allChecks []check.Check
	for _, config := range configs {
		if !isCheckConfig(config) {
			// skip non c configs.
			continue
		}
		configDigest := config.Digest()
		checks, err := ac.getChecks(config)
		if err != nil {
			log.Errorf("Unable to load the c: %v", err)
			continue
		}
		for _, c := range checks {
			allChecks = append(allChecks, c)
			if populateCache {
				// store the checks we schedule for this config locally
				ac.configToChecks[configDigest] = append(ac.configToChecks[configDigest], c.ID())
				ac.checkToConfig[c.ID()] = configDigest
			}
		}
	}

	return allChecks
}

// isCheckConfig returns true if the config is a check configuration,
// this method should be moved to pkg/collector/check while removing the check related-code from the autodiscovery package.
func isCheckConfig(config integration.Config) bool {
	return config.MetricConfig != nil || len(config.Instances) > 0
}

// schedule takes a slice of checks and schedule them
func (ac *AutoConfig) schedule(checks []check.Check) {
	for _, c := range checks {
		log.Infof("Scheduling check %s", c)
		_, err := ac.collector.RunCheck(c)
		if err != nil {
			log.Errorf("Unable to run Check %s: %v", c, err)
			errorStats.setRunError(c.ID(), err.Error())
			continue
		}
	}
}

// resolve loads and resolves a given config into a slice of resolved configs
func (ac *AutoConfig) resolve(config integration.Config) []integration.Config {
	var configs []integration.Config

	log.Infof("Resolving %s", config.Name)
	// add default metrics to collect to JMX checks
	if check.CollectDefaultMetrics(config) {
		metrics, ok := ac.nameToJMXMetrics[config.Name]
		if !ok {
			log.Infof("%s doesn't have an additional metric configuration file: not collecting default metrics", config.Name)
		} else if err := config.AddMetrics(metrics); err != nil {
			log.Infof("Unable to add default metrics to collect to %s check: %s", config.Name, err)
		}
	}

	if config.IsTemplate() {
		log.Infof("Resolving %s as template", config.Name)
		// store the template in the cache in any case
		if err := ac.templateCache.Set(config); err != nil {
			log.Errorf("Unable to store Check configuration in the cache: %s", err)
		}

		// try to resolve the template
		log.Infof("Try to resolve template %s", config.Name)
		resolvedConfigs := ac.configResolver.ResolveTemplate(config)
		if len(resolvedConfigs) == 0 {
			e := fmt.Sprintf("Can't resolve the template for %s at this moment.", config.Name)
			errorStats.setResolveWarning(config.Name, e)
			log.Infof(e)
			return configs
		}
		log.Infof("Resolved template %s: %d", config.Name, len(resolvedConfigs))
		errorStats.removeResolveWarnings(config.Name)

		// each template can resolve to multiple configs
		for _, config := range resolvedConfigs {
			config, err := decryptConfig(config)
			if err != nil {
				log.Errorf("Dropping conf for '%s': %s", config.Name, err.Error())
				continue
			}
			configs = append(configs, config)
		}
		log.Infof("Returning %d configs", len(configs))
		return configs
	}
	log.Infof("Resolving %s NOT as template", config.String())
	config, err := decryptConfig(config)
	if err != nil {
		log.Errorf("Dropping conf for '%s': %s", config.Name, err.Error())
		return configs
	}
	configs = append(configs, config)

	// store non template configs in the AC
	//ac.m.Lock()
	ac.loadedConfigs[config.Digest()] = config
	//ac.m.Unlock()
	return configs
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

func decryptConfig(conf integration.Config) (integration.Config, error) {
	var err error

	// init_config
	conf.InitConfig, err = secrets.Decrypt(conf.InitConfig)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets in 'init_config': %s", err)
	}

	// instances
	for idx := range conf.Instances {
		conf.Instances[idx], err = secrets.Decrypt(conf.Instances[idx])
		if err != nil {
			return conf, fmt.Errorf("error while decrypting secrets in an instance: %s", err)
		}
	}

	// metrics
	conf.MetricConfig, err = secrets.Decrypt(conf.MetricConfig)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets in 'metrics': %s", err)
	}

	// logs
	conf.LogsConfig, err = secrets.Decrypt(conf.LogsConfig)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets 'logs': %s", err)
	}

	return conf, nil
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
				ac.health.Deregister()
				return
			case <-ac.health.C:
			case <-ac.configsPollTicker.C:
				ac.m.Lock()

				// invoke Collect on the known providers
				for _, pd := range ac.providers {
					// skip providers that don't want to be polled
					if !pd.poll {
						continue
					}

					// Check if the CPupdate cache is up to date. Fill it and trigger a Collect() if outdated.
					log.Infof("Processing %q", pd.provider)
					upToDate, err := pd.provider.IsUpToDate()
					if err != nil {
						log.Errorf("cache processing of %v failed: %v", pd.provider.String(), err)
					}
					if upToDate == true {
						log.Debugf("No modifications in the templates stored in %q ", pd.provider.String())
						continue
					}

					// retrieve the list of newly added configurations as well
					// as removed configurations
					newConfigs, removedConfigs := ac.collect(pd)
					log.Infof("evaluating provider %s, removing %s, adding %s", pd.provider.String(), removedConfigs, newConfigs)
					ac.processRemovedConfigs(removedConfigs)

					// TODO: move to check scheduler
					for _, config := range newConfigs {
						log.Infof("processing the new config %s", config)
						config.Provider = pd.provider.String()
						resolvedConfigs := ac.resolve(config)
						log.Infof("resolved to %s", resolvedConfigs)
						checks := ac.getChecksFromConfigs(resolvedConfigs, true)
						log.Infof("collected the checks %s", checks)
						ac.schedule(checks)
					}
				}
				ac.m.Unlock()
			}
		}
	}()
}

// TODO: move to check scheduler
func (ac *AutoConfig) processRemovedConfigs(removedConfigs []integration.Config) {
	// Process removed configs first to handle the case where a
	// container churn would result in the same configuration hash.
	for _, config := range removedConfigs {
		log.Infof("Started removing %s", config)
		if !isCheckConfig(config) {
			// skip non elt configs.
			continue
		}
		// unschedule all the possible checks corresponding to this config
		digest := config.Digest()
		stopped := map[check.ID]struct{}{}
		for _, elt := range ac.configToChecks[digest] {
			// `StopCheck` might time out so we don't risk to block
			// the polling loop forever
			err := ac.collector.StopCheck(elt)
			if err != nil {
				log.Errorf("Error stopping check %s: %s", elt, err)
				errorStats.setRunError(elt, err.Error())
				continue
			}
			stopped[elt] = struct{}{}
		}

		// remove the entry from `configToChecks`
		if len(stopped) == len(ac.configToChecks[digest]) {
			// we managed to stop all the checks for this config
			delete(ac.configToChecks, digest)
			delete(ac.configResolver.configToService, digest)
			delete(ac.loadedConfigs, digest)
		} else {
			// keep the checks we failed to stop in `configToChecks`
			var dangling []check.ID
			for _, elt := range ac.configToChecks[digest] {
				if _, found := stopped[elt]; !found {
					dangling = append(dangling, elt)
				}
			}
			ac.configToChecks[digest] = dangling
		}

		// if the config is a template, remove it from the cache
		if config.IsTemplate() {
			log.Infof("Config %s is template, deleting", config)
			ac.templateCache.Del(config)
		}
		log.Infof("Finished removing %s", config)
	}
}

// collect is just a convenient wrapper to fetch configurations from a provider and
// see what changed from the last time we called Collect().
func (ac *AutoConfig) collect(pd *providerDescriptor) ([]integration.Config, []integration.Config) {
	var newConf []integration.Config
	var removedConf []integration.Config
	old := pd.configs

	fetched, err := pd.provider.Collect()
	if err != nil {
		log.Errorf("Unable to collect configurations from provider %s: %s", pd.provider, err)
		return nil, nil
	}

	for _, c := range fetched {
		log.Infof("c.ADIdentifiers is %s", c.ADIdentifiers) // REMOVE
		if !pd.contains(&c) {
			newConf = append(newConf, c)
			log.Infof("New config: %s", c)
			continue
		}
		log.Infof("provider contains %s", c)
		// Check the freshness of c. Reschedule if necessary.
		if tagger.OutdatedTags(c.ADIdentifiers) {
			log.Infof("Starting rescheduling for %s", c)
			removedConf = append(removedConf, c)
			newConf = append(newConf, c)
			continue
		}
		log.Infof("No need to reschedule %s", c)
	}

	pd.configs = fetched
	for _, c := range old {
		if !pd.contains(&c) {
			removedConf = append(removedConf, c)
		}
	}
	log.Infof("%v: collected %d new configurations, removed %d", pd.provider, len(newConf), len(removedConf))
	return newConf, removedConf
}

// getChecks takes a check configuration and returns a slice of Check instances
// along with any error it might happen during the process
func (ac *AutoConfig) getChecks(config integration.Config) ([]check.Check, error) {
	for _, loader := range ac.loaders {
		res, err := loader.Load(config)
		if err == nil {
			log.Debugf("%v: successfully loaded check '%s'", loader, config.Name)
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

// GetLoadedConfigs returns configs loaded
func (ac *AutoConfig) GetLoadedConfigs() map[string]integration.Config {
	configsCopy := make(map[string]integration.Config)
	ac.m.RLock()
	defer ac.m.RUnlock()
	for k, v := range ac.loadedConfigs {
		configsCopy[k] = v
	}
	return configsCopy
}

// GetUnresolvedTemplates returns templates in cache yet to be resolved
func (ac *AutoConfig) GetUnresolvedTemplates() map[string]integration.Config {
	return ac.templateCache.GetUnresolvedTemplates()
}

// unschedule removes the check to config cache mapping
func (ac *AutoConfig) unschedule(id check.ID) {
	delete(ac.checkToConfig, id)
}

// check if the descriptor contains the Config passed
func (pd *providerDescriptor) contains(c *integration.Config) bool {
	for _, config := range pd.configs {
		if !config.Equal(c) {
			log.Infof("is c: %s equal to config: %s", c, config)
			continue
		}
		return true
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

// GetResolveWarnings get the resolve warnings/errors
func GetResolveWarnings() map[string][]string {
	return errorStats.getResolveWarnings()
}

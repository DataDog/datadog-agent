// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package autodiscovery

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
	configsPollIntl       = 10 * time.Second
	listenerCandidateIntl = 30 * time.Second
	acErrors              *expvar.Map
	errorStats            = newAcErrorStats()
)

func init() {
	acErrors = expvar.NewMap("autoconfig")
	acErrors.Set("ConfigErrors", expvar.Func(func() interface{} {
		return errorStats.getConfigErrors()
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

// AutoConfig is responsible to collect integrations configurations from
// different sources and then schedule or unschedule them.
// It owns and orchestrates several key modules:
//  - it owns a reference to the `collector.Collector` that it uses to schedule checks when template or container updates warrant them
//  - it holds a list of `providers.ConfigProvider`s and poll them according to their policy
//  - it holds a list of `check.Loader`s to load configurations into `Check` objects
//  - it holds a list of `listeners.ServiceListener`s` used to listen to container lifecycle events
//  - it runs the `ConfigResolver` that resolves a configuration template to an actual configuration based on data it extracts from a service that matches it the template
//
// Notice the `AutoConfig` public API speaks in terms of `integration.Config`,
// meaning that you cannot use it to schedule integrations instances directly.
type AutoConfig struct {
	providers          []*providerDescriptor
	templateCache      *TemplateCache
	listeners          []listeners.ServiceListener
	listenerCandidates map[string]listeners.ServiceListenerFactory
	listenerRetryStop  chan struct{}
	configResolver     *ConfigResolver
	configsPollTicker  *time.Ticker
	scheduler          scheduler.Scheduler
	pollerStop         chan struct{}
	pollerActive       bool
	health             *health.Handle
	store              *store
	m                  sync.RWMutex
}

// NewAutoConfig creates an AutoConfig instance.
func NewAutoConfig(scheduler scheduler.Scheduler) *AutoConfig {
	ac := &AutoConfig{
		providers:          make([]*providerDescriptor, 0, 5),
		listenerCandidates: make(map[string]listeners.ServiceListenerFactory),
		listenerRetryStop:  nil, // We'll open it if needed
		templateCache:      NewTemplateCache(),
		pollerStop:         make(chan struct{}),
		store:              newStore(),
		health:             health.Register("ad-autoconfig"),
		scheduler:          scheduler,
	}
	ac.configResolver = newConfigResolver(ac, ac.templateCache)
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
		ac.pollerStop <- struct{}{}
		ac.pollerActive = false
	}

	// stop the meta scheduler
	ac.scheduler.Stop()

	// stop the config resolver
	if ac.configResolver != nil {
		ac.configResolver.Stop()
	}

	// stop the listener retry logic if running
	if ac.listenerRetryStop != nil {
		ac.listenerRetryStop <- struct{}{}
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

// LoadAndRun loads all of the integration configs it can find
// and schedules them. Should always be run once so providers
// that don't need polling will be queried at least once
func (ac *AutoConfig) LoadAndRun() {
	resolvedConfigs := ac.GetAllConfigs()
	ac.schedule(resolvedConfigs)
}

// GetAllConfigs queries all the providers and returns all the integration
// configurations found, resolving the ones it can
func (ac *AutoConfig) GetAllConfigs() []integration.Config {
	var resolvedConfigs []integration.Config

	for _, pd := range ac.providers {
		cfgs, err := pd.provider.Collect()
		if err != nil {
			log.Debugf("Unexpected error returned when collecting configurations from provider %v: %v", pd.provider, err)
		}

		if fileConfPd, ok := pd.provider.(*providers.FileConfigProvider); ok {
			var goodConfs []integration.Config
			for _, cfg := range cfgs {
				// JMX checks can have 2 YAML files: one containing the metrics to collect, one containing the
				// instance configuration
				// If the file provider finds any of these metric YAMLs, we store them in a map for future access
				if cfg.MetricConfig != nil {
					// We don't want to save metric files, it's enough to store them in the map
					ac.store.setJMXMetricsForConfigName(cfg.Name, cfg.MetricConfig)
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

// schedule takes a slice of configs and schedule them
func (ac *AutoConfig) schedule(configs []integration.Config) {
	ac.scheduler.Schedule(configs)
}

// resolve loads and resolves a given config into a slice of resolved configs
func (ac *AutoConfig) resolve(config integration.Config) []integration.Config {
	var configs []integration.Config

	// add default metrics to collect to JMX checks
	if check.CollectDefaultMetrics(config) {
		metrics := ac.store.getJMXMetricsForConfigName(config.Name)
		if len(metrics) == 0 {
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
			e := fmt.Sprintf("Can't resolve the template for %s at this moment.", config.Name)
			errorStats.setResolveWarning(config.Name, e)
			log.Debug(e)
			return configs
		}
		errorStats.removeResolveWarnings(config.Name)

		// each template can resolve to multiple configs
		for _, config := range resolvedConfigs {
			config, err := decryptConfig(config)
			if err != nil {
				log.Errorf("Dropping conf for %q: %s", config.Name, err.Error())
				continue
			}
			configs = append(configs, config)
		}
		return configs
	}
	config, err := decryptConfig(config)
	if err != nil {
		log.Errorf("Dropping conf for '%s': %s", config.Name, err.Error())
		return configs
	}
	configs = append(configs, config)

	// store non template configs in the AC
	ac.store.setLoadedConfig(config)
	return configs
}

// AddListeners tries to initialise the listeners listed in the given configs. A first
// try is done synchronously. If a listener fails with a ErrWillRetry, the initialization
// will be re-triggered later until success or ErrPermaFail.
func (ac *AutoConfig) AddListeners(listenerConfigs []config.Listeners) {
	ac.addListenerCandidates(listenerConfigs)
	remaining := ac.initListenerCandidates()
	if remaining == false {
		return
	}

	// Start the retry logic if we have remaining candidates and it is not already running
	ac.m.Lock()
	defer ac.m.Unlock()
	if ac.listenerRetryStop == nil {
		ac.listenerRetryStop = make(chan struct{})
		go ac.retryListenerCandidates()
	}
}

func (ac *AutoConfig) addListenerCandidates(listenerConfigs []config.Listeners) {
	ac.m.Lock()
	defer ac.m.Unlock()

	for _, c := range listenerConfigs {
		factory, ok := listeners.ServiceListenerFactories[c.Name]
		if !ok {
			// Factory has not been registered.
			log.Warnf("Listener %s was not registered", c.Name)
			continue
		}
		log.Debugf("Listener %s was registered", c.Name)
		ac.listenerCandidates[c.Name] = factory
	}
}

func (ac *AutoConfig) initListenerCandidates() bool {
	ac.m.Lock()
	defer ac.m.Unlock()

	for name, factory := range ac.listenerCandidates {
		listener, err := factory()
		switch {
		case err == nil:
			// Init successful, let's start listening
			log.Infof("%s listener successfully started", name)
			ac.listeners = append(ac.listeners, listener)
			listener.Listen(ac.configResolver.newService, ac.configResolver.delService)
			delete(ac.listenerCandidates, name)
		case retry.IsErrWillRetry(err):
			// Log an info and keep in candidates
			log.Infof("%s listener cannot start, will retry: %s", name, err)
		default:
			// Log an error and remove from candidates
			log.Errorf("%s listener cannot start: %s", name, err)
			delete(ac.listenerCandidates, name)
		}
	}

	return len(ac.listenerCandidates) > 0
}

func (ac *AutoConfig) retryListenerCandidates() {
	retryTicker := time.NewTicker(listenerCandidateIntl)
	defer func() {
		// Stop ticker
		retryTicker.Stop()
		// Cleanup channel before exiting so that we can re-start the routine later
		ac.m.Lock()
		defer ac.m.Unlock()
		close(ac.listenerRetryStop)
		ac.listenerRetryStop = nil
	}()

	for {
		select {
		case <-ac.listenerRetryStop:
			return
		case <-retryTicker.C:
			remaining := ac.initListenerCandidates()
			if !remaining {
				return
			}
		}
	}
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
			case <-ac.pollerStop:
				if ac.configsPollTicker != nil {
					ac.configsPollTicker.Stop()
				}
				ac.health.Deregister()
				return
			case <-ac.health.C:
			case <-ac.configsPollTicker.C:
				// check if services tags are up to date
				var servicesToRefresh []listeners.Service
				// locking the configresolver to loop on services
				ac.configResolver.m.Lock()
				for _, service := range ac.configResolver.services {
					previousHash := ac.store.getTagsHashForService(service.GetEntity())
					currentHash := tagger.GetEntityHash(service.GetEntity())
					if currentHash != previousHash {
						ac.store.setTagsHashForService(service.GetEntity(), currentHash)
						if previousHash != "" {
							// only refresh service if we already had a hash to avoid resetting it
							servicesToRefresh = append(servicesToRefresh, service)
						}
					}
				}
				ac.configResolver.m.Unlock()
				for _, service := range servicesToRefresh {
					log.Debugf("Tags changed for service %s, rescheduling associated checks if any", service.GetEntity())
					ac.configResolver.processDelService(service)
					ac.configResolver.processNewService(service)
				}
				// invoke Collect on the known providers
				for _, pd := range ac.providers {
					// skip providers that don't want to be polled
					if !pd.poll {
						continue
					}

					// Check if the CPupdate cache is up to date. Fill it and trigger a Collect() if outdated.
					upToDate, err := pd.provider.IsUpToDate()
					if err != nil {
						log.Errorf("cache processing of %v configuration provider failed: %v", pd.provider, err)
					}
					if upToDate == true {
						log.Debugf("No modifications in the templates stored in %v configuration provider", pd.provider)
						continue
					}

					// retrieve the list of newly added configurations as well
					// as removed configurations
					newConfigs, removedConfigs := ac.collect(pd)
					// Process removed configs first to handle the case where a
					// container churn would result in the same configuration hash.
					ac.processRemovedConfigs(removedConfigs)

					for _, config := range newConfigs {
						config.Provider = pd.provider.String()
						resolvedConfigs := ac.resolve(config)
						ac.schedule(resolvedConfigs)
					}
				}
			}
		}
	}()
}

func (ac *AutoConfig) processRemovedConfigs(configs []integration.Config) {
	ac.scheduler.Unschedule(configs)
	for _, c := range configs {
		delete(ac.configResolver.configToService, c.Digest())
		ac.store.removeLoadedConfig(c)
		// if the config is a template, remove it from the cache
		if c.IsTemplate() {
			ac.templateCache.Del(c)
		}
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
		if !pd.contains(&c) {
			newConf = append(newConf, c)
		}
	}

	pd.configs = fetched
	for _, c := range old {
		if !pd.contains(&c) {
			removedConf = append(removedConf, c)
		}
	}
	if len(newConf) > 0 || len(removedConf) > 0 {
		log.Infof("%v provider: collected %d new configurations, removed %d", pd.provider, len(newConf), len(removedConf))
	} else {
		log.Debugf("%v provider: no configuration change", pd.provider)
	}
	return newConf, removedConf
}

// GetLoadedConfigs returns configs loaded
func (ac *AutoConfig) GetLoadedConfigs() map[string]integration.Config {
	return ac.store.getLoadedConfigs()
}

// GetUnresolvedTemplates returns templates in cache yet to be resolved
func (ac *AutoConfig) GetUnresolvedTemplates() map[string]integration.Config {
	return ac.templateCache.GetUnresolvedTemplates()
}

// check if the descriptor contains the Config passed
func (pd *providerDescriptor) contains(c *integration.Config) bool {
	for _, config := range pd.configs {
		if config.Equal(c) {
			return true
		}
	}
	return false
}

// GetConfigErrors gets the config errors
func GetConfigErrors() map[string]string {
	return errorStats.getConfigErrors()
}

// GetResolveWarnings get the resolve warnings/errors
func GetResolveWarnings() map[string][]string {
	return errorStats.getResolveWarnings()
}

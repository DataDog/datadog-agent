// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package autodiscovery

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/configresolver"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/providers"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/secrets"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

var (
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

// AutoConfig is responsible to collect integrations configurations from
// different sources and then schedule or unschedule them.
// It owns and orchestrates several key modules:
//  - it owns a reference to the `collector.Collector` that it uses to schedule checks when template or container updates warrant them
//  - it holds a list of `providers.ConfigProvider`s and poll them according to their policy
//  - it holds a list of `check.Loader`s to load configurations into `Check` objects
//  - it holds a list of `listeners.ServiceListener`s` used to listen to container lifecycle events
//  - it uses the `ConfigResolver` that resolves a configuration template to an actual configuration based on a service matching the template
//
// Notice the `AutoConfig` public API speaks in terms of `integration.Config`,
// meaning that you cannot use it to schedule integrations instances directly.
type AutoConfig struct {
	providers          []*configPoller
	listeners          []listeners.ServiceListener
	listenerCandidates map[string]listeners.ServiceListenerFactory
	listenerRetryStop  chan struct{}
	scheduler          *scheduler.MetaScheduler
	listenerStop       chan struct{}
	healthListening    *health.Handle
	newService         chan listeners.Service
	delService         chan listeners.Service
	store              *store
	m                  sync.RWMutex
}

// NewAutoConfig creates an AutoConfig instance.
func NewAutoConfig(scheduler *scheduler.MetaScheduler) *AutoConfig {
	ac := &AutoConfig{
		providers:          make([]*configPoller, 0, 9),
		listenerCandidates: make(map[string]listeners.ServiceListenerFactory),
		listenerRetryStop:  nil, // We'll open it if needed
		listenerStop:       make(chan struct{}),
		healthListening:    health.RegisterLiveness("ad-servicelistening"),
		newService:         make(chan listeners.Service),
		delService:         make(chan listeners.Service),
		store:              newStore(),
		scheduler:          scheduler,
	}
	// We need to listen to the service channels before anything is sent to them
	go ac.serviceListening()
	return ac
}

// serviceListening is the main management goroutine for services.
// It waits for service events to trigger template resolution and
// checks the tags on existing services are up to date.
func (ac *AutoConfig) serviceListening() {
	tagFreshnessTicker := time.NewTicker(15 * time.Second) // we can miss tags for one run
	defer tagFreshnessTicker.Stop()

	for {
		select {
		case <-ac.listenerStop:
			ac.healthListening.Deregister() //nolint:errcheck
			return
		case <-ac.healthListening.C:
		case svc := <-ac.newService:
			ac.processNewService(svc)
		case svc := <-ac.delService:
			ac.processDelService(svc)
		case <-tagFreshnessTicker.C:
			ac.checkTagFreshness()
		}
	}
}

func (ac *AutoConfig) checkTagFreshness() {
	// check if services tags are up to date
	var servicesToRefresh []listeners.Service
	for _, service := range ac.store.getServices() {
		previousHash := ac.store.getTagsHashForService(service.GetTaggerEntity())
		currentHash := tagger.GetEntityHash(service.GetTaggerEntity())
		// Since an empty hash is a valid value, and we are not able to differentiate
		// an empty tagger or store with an empty value.
		// So we only look at the difference between current and previous
		if currentHash != previousHash {
			ac.store.setTagsHashForService(service.GetTaggerEntity(), currentHash)
			servicesToRefresh = append(servicesToRefresh, service)
		}
	}
	for _, service := range servicesToRefresh {
		log.Debugf("Tags changed for service %s, rescheduling associated checks if any", service.GetTaggerEntity())
		ac.processDelService(service)
		ac.processNewService(service)
	}
}

// Stop just shuts down AutoConfig in a clean way.
// AutoConfig is not supposed to be restarted, so this is expected
// to be called only once at program exit.
func (ac *AutoConfig) Stop() {
	ac.m.Lock()
	defer ac.m.Unlock()

	// stop polled config providers
	for _, pd := range ac.providers {
		pd.stop()
	}

	// stop the service listener
	ac.listenerStop <- struct{}{}

	// stop the meta scheduler
	ac.scheduler.Stop()

	// stop the listener retry logic if running
	if ac.listenerRetryStop != nil {
		ac.listenerRetryStop <- struct{}{}
	}

	// stop all the listeners
	for _, l := range ac.listeners {
		l.Stop()
	}
}

// AddConfigProvider adds a new configuration provider to AutoConfig.
// Callers must pass a flag to indicate whether the configuration provider
// expects to be polled and at which interval or it's fine for it to be invoked only once in the
// Agent lifetime.
// If the config provider is polled, the routine is scheduled right away
func (ac *AutoConfig) AddConfigProvider(provider providers.ConfigProvider, shouldPoll bool, pollInterval time.Duration) {
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

	pd := newConfigPoller(provider, shouldPoll, pollInterval)
	ac.providers = append(ac.providers, pd)
	pd.start(ac)
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
			rc := ac.processNewConfig(config)
			resolvedConfigs = append(resolvedConfigs, rc...)
		}
	}

	return resolvedConfigs
}

// schedule takes a slice of configs and schedule them
func (ac *AutoConfig) schedule(configs []integration.Config) {
	ac.scheduler.Schedule(configs)
}

// unschedule takes a slice of configs and unschedule them
func (ac *AutoConfig) unschedule(configs []integration.Config) {
	ac.scheduler.Unschedule(configs)
}

// processNewConfig store (in template cache) and resolves a given config into a slice of resolved configs
func (ac *AutoConfig) processNewConfig(config integration.Config) []integration.Config {
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
		if err := ac.store.templateCache.Set(config); err != nil {
			log.Errorf("Unable to store Check configuration in the cache: %s", err)
		}

		// try to resolve the template
		resolvedConfigs := ac.resolveTemplate(config)
		if len(resolvedConfigs) == 0 {
			e := fmt.Sprintf("Can't resolve the template for %s at this moment.", config.Name)
			errorStats.setResolveWarning(config.Name, e)
			log.Debug(e)
			return configs
		}

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
			listener.Listen(ac.newService, ac.delService)
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

// AddScheduler allows to register a new scheduler to receive configurations.
// Previously emitted configurations can be replayed with the replayConfigs flag.
func (ac *AutoConfig) AddScheduler(name string, s scheduler.Scheduler, replayConfigs bool) {
	ac.m.Lock()
	defer ac.m.Unlock()

	ac.scheduler.Register(name, s)
	if !replayConfigs {
		return
	}

	var configs []integration.Config
	for _, c := range ac.store.getLoadedConfigs() {
		configs = append(configs, c)
	}
	s.Schedule(configs)
}

// RemoveScheduler allows to remove a scheduler from the AD system.
func (ac *AutoConfig) RemoveScheduler(name string) {
	ac.scheduler.Deregister(name)
}

func decryptConfig(conf integration.Config) (integration.Config, error) {
	var err error

	// init_config
	conf.InitConfig, err = secrets.Decrypt(conf.InitConfig, conf.Name)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets in 'init_config': %s", err)
	}

	// instances
	for idx := range conf.Instances {
		conf.Instances[idx], err = secrets.Decrypt(conf.Instances[idx], conf.Name)
		if err != nil {
			return conf, fmt.Errorf("error while decrypting secrets in an instance: %s", err)
		}
	}

	// metrics
	conf.MetricConfig, err = secrets.Decrypt(conf.MetricConfig, conf.Name)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets in 'metrics': %s", err)
	}

	// logs
	conf.LogsConfig, err = secrets.Decrypt(conf.LogsConfig, conf.Name)
	if err != nil {
		return conf, fmt.Errorf("error while decrypting secrets 'logs': %s", err)
	}

	return conf, nil
}

func (ac *AutoConfig) processRemovedConfigs(configs []integration.Config) {
	ac.unschedule(configs)
	for _, c := range configs {
		ac.store.removeLoadedConfig(c)
	}
}

func (ac *AutoConfig) removeConfigTemplates(configs []integration.Config) {
	for _, c := range configs {
		if c.IsTemplate() {
			// Remove the resolved configurations
			tplDigest := c.Digest()
			configs := ac.store.getConfigsForTemplate(tplDigest)
			ac.store.removeConfigsForTemplate(tplDigest)
			ac.processRemovedConfigs(configs)

			// Remove template from the cache
			err := ac.store.templateCache.Del(c)
			if err != nil {
				log.Debugf("Could not delete template: %v", err)
			}
		}
	}
}

// resolveTemplate attempts to resolve a configuration template using the AD
// identifiers in the `integration.Config` struct to match a Service.
//
// The function might return more than one configuration for a single template,
// for example when the `ad_identifiers` section of a config.yaml file contains
// multiple entries, or when more than one Service has the same identifier,
// e.g. 'redis'.
//
// The function might return an empty list in the case the configuration has a
// list of Autodiscovery identifiers for services that are unknown to the
// resolver at this moment.
func (ac *AutoConfig) resolveTemplate(tpl integration.Config) []integration.Config {
	// use a map to dedupe configurations
	resolvedSet := map[string]integration.Config{}

	// go through the AD identifiers provided by the template
	for _, id := range tpl.ADIdentifiers {
		// check out whether any service we know has this identifier
		serviceIds, found := ac.store.getServiceEntitiesForADID(id)
		if !found {
			s := fmt.Sprintf("No service found with this AD identifier: %s", id)
			errorStats.setResolveWarning(tpl.Name, s)
			log.Debugf(s)
			continue
		}

		for serviceID := range serviceIds {
			svc := ac.store.getServiceForEntity(serviceID)
			if svc == nil {
				log.Warnf("Service %s was removed before we could resolve its config", serviceID)
				continue
			}
			resolvedConfig, err := ac.resolveTemplateForService(tpl, svc)
			if err != nil {
				continue
			}
			resolvedSet[resolvedConfig.Digest()] = resolvedConfig
		}
	}

	// build the slice of configs to return
	var resolved []integration.Config
	for _, v := range resolvedSet {
		resolved = append(resolved, v)
	}

	return resolved
}

// resolveTemplateForService calls the config resolver for the template against the service
// and stores the resolved config and service mapping if successful
func (ac *AutoConfig) resolveTemplateForService(tpl integration.Config, svc listeners.Service) (integration.Config, error) {
	resolvedConfig, err := configresolver.Resolve(tpl, svc)
	if err != nil {
		newErr := fmt.Errorf("error resolving template %s for service %s: %v", tpl.Name, svc.GetEntity(), err)
		errorStats.setResolveWarning(tpl.Name, newErr.Error())
		return tpl, log.Warn(newErr)
	}
	ac.store.setLoadedConfig(resolvedConfig)
	ac.store.addConfigForService(svc.GetEntity(), resolvedConfig)
	ac.store.addConfigForTemplate(tpl.Digest(), resolvedConfig)
	ac.store.setTagsHashForService(
		svc.GetTaggerEntity(),
		tagger.GetEntityHash(svc.GetTaggerEntity()),
	)
	errorStats.removeResolveWarnings(tpl.Name)
	return resolvedConfig, nil
}

// GetLoadedConfigs returns configs loaded
func (ac *AutoConfig) GetLoadedConfigs() map[string]integration.Config {
	if ac == nil || ac.store == nil {
		log.Error("Autoconfig store not initialized")
		return map[string]integration.Config{}
	}
	return ac.store.getLoadedConfigs()
}

// GetUnresolvedTemplates returns templates in cache yet to be resolved
func (ac *AutoConfig) GetUnresolvedTemplates() map[string][]integration.Config {
	return ac.store.templateCache.GetUnresolvedTemplates()
}

// GetConfigErrors gets the config errors
func GetConfigErrors() map[string]string {
	return errorStats.getConfigErrors()
}

// GetResolveWarnings get the resolve warnings/errors
func GetResolveWarnings() map[string][]string {
	return errorStats.getResolveWarnings()
}

// processNewService takes a service, tries to match it against templates and
// triggers scheduling events if it finds a valid config for it.
func (ac *AutoConfig) processNewService(svc listeners.Service) {
	// in any case, register the service and store its tag hash
	ac.store.setServiceForEntity(svc, svc.GetEntity())
	ac.store.setTagsHashForService(
		svc.GetTaggerEntity(),
		tagger.GetEntityHash(svc.GetTaggerEntity()),
	)

	// get all the templates matching service identifiers
	var templates []integration.Config
	ADIdentifiers, err := svc.GetADIdentifiers()
	if err != nil {
		log.Errorf("Failed to get AD identifiers for service %s, it will not be monitored - %s", svc.GetEntity(), err)
		return
	}
	for _, adID := range ADIdentifiers {
		// map the AD identifier to this service for reverse lookup
		ac.store.setADIDForServices(adID, svc.GetEntity())
		tpls, err := ac.store.templateCache.Get(adID)
		if err != nil {
			log.Debugf("Unable to fetch templates from the cache: %v", err)
		}
		templates = append(templates, tpls...)
	}

	for _, template := range templates {
		// resolve the template
		resolvedConfig, err := ac.resolveTemplateForService(template, svc)
		if err != nil {
			continue
		}

		// ask the Collector to schedule the checks
		ac.schedule([]integration.Config{resolvedConfig})
	}
	// FIXME: schedule new services as well
	ac.schedule([]integration.Config{
		{
			LogsConfig:      integration.Data{},
			Entity:          svc.GetEntity(),
			TaggerEntity:    svc.GetTaggerEntity(),
			CreationTime:    svc.GetCreationTime(),
			MetricsExcluded: svc.HasFilter(containers.MetricsFilter),
			LogsExcluded:    svc.HasFilter(containers.LogsFilter),
		},
	})

}

// processDelService takes a service, stops its associated checks, and updates the cache
func (ac *AutoConfig) processDelService(svc listeners.Service) {
	ac.store.removeServiceForEntity(svc.GetEntity())
	configs := ac.store.getConfigsForService(svc.GetEntity())
	ac.store.removeConfigsForService(svc.GetEntity())
	ac.processRemovedConfigs(configs)
	ac.store.removeTagsHashForService(svc.GetTaggerEntity())
	// FIXME: unschedule remove services as well
	ac.unschedule([]integration.Config{
		{
			LogsConfig:      integration.Data{},
			Entity:          svc.GetEntity(),
			TaggerEntity:    svc.GetTaggerEntity(),
			CreationTime:    svc.GetCreationTime(),
			MetricsExcluded: svc.HasFilter(containers.MetricsFilter),
			LogsExcluded:    svc.HasFilter(containers.LogsFilter),
		},
	})
}

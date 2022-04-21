// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/configresolver"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// configChanges contains the changes that occurred due to an event in a
// configManager.
type configChanges struct {

	// schedule contains configs that should be scheduled as a result of this
	// event.
	schedule []integration.Config

	// unschedule contains configs that should be unscheduled as a result of
	// this event.
	unschedule []integration.Config
}

// scheduleConfig adds a config to `schedule`
func (c *configChanges) scheduleConfig(config integration.Config) {
	c.schedule = append(c.schedule, config)
}

// unscheduleConfig adds a config to `unschedule`
func (c *configChanges) unscheduleConfig(config integration.Config) {
	c.unschedule = append(c.unschedule, config)
}

// isEmpty determines whether this set of changes is empty
func (c *configChanges) isEmpty() bool {
	return len(c.schedule) == 0 && len(c.unschedule) == 0
}

// merge merges the given configChanges into this one.
func (c *configChanges) merge(other configChanges) {
	c.schedule = append(c.schedule, other.schedule...)
	c.unschedule = append(c.unschedule, other.unschedule...)
}

// configManager implememnts the logic of handling additions and removals of
// configs (which may or may not be templates) and services, and reconciling
// those together to resolve templates.
//
// This type is threadsafe, internally using a mutex to serialize operations.
type configManager interface {
	// processNewService handles a new service, with the given AD identifiers
	processNewService(adIdentifiers []string, svc listeners.Service) configChanges

	// processDelService handles removal of a service, unscheduling any configs
	// that had been resolved for it.
	processDelService(svc listeners.Service) configChanges

	// processNewConfig handles a new config
	processNewConfig(config integration.Config) configChanges

	// processDelConfigs handles removal of a config, unscheduling the config
	// itself or, if it is a template, any configs resolved from it.  Note that
	// this applies to a slice of configs, where the other methods in this
	// interface apply to only one config.
	processDelConfigs(configs []integration.Config) configChanges

	// mapOverLoadedConfigs calls the given function with a map of all
	// loaded configs (those which have been scheduled but not unscheduled).
	// The call is made with the manager's lock held, so callers should perform
	// minimal work within f.
	mapOverLoadedConfigs(func(map[string]integration.Config))
}

// simpleConfigManager implements the "simple" config manager that reconciles
// services and templates without any priority, using a store as a backend.
type simpleConfigManager struct {
	// m synchronizes all operations on this struct.
	m sync.Mutex

	// store contains the data tracked by this manager.
	store *store
}

// newSimpleConfigManager creates a new, empty simpleConfigManager.
func newSimpleConfigManager() configManager {
	return &simpleConfigManager{
		store: newStore(),
	}
}

// processNewService implements configManager#processNewService.
func (cm *simpleConfigManager) processNewService(adIdentifiers []string, svc listeners.Service) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()

	cm.store.setServiceForEntity(svc, svc.GetServiceID())

	var templates []integration.Config
	for _, adID := range adIdentifiers {
		// map the AD identifier to this service for reverse lookup
		cm.store.setADIDForServices(adID, svc.GetServiceID())
		tpls, err := cm.store.templateCache.get(adID)
		if err != nil {
			log.Debugf("Unable to fetch templates from the cache: %v", err)
		}
		templates = append(templates, tpls...)
	}

	resolvedSet := map[string]integration.Config{}
	for _, template := range templates {
		// resolve the template
		resolvedConfig, err := cm.resolveTemplateForService(template, svc)
		if err != nil {
			continue
		}

		resolvedSet[resolvedConfig.Digest()] = resolvedConfig
	}

	// build the config changes to return
	changes := configChanges{}
	for _, v := range resolvedSet {
		changes.scheduleConfig(v)
	}

	return changes
}

// processDelService implements configManager#processDelService.
func (cm *simpleConfigManager) processDelService(svc listeners.Service) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()
	changes := configChanges{}

	cm.store.removeServiceForEntity(svc.GetServiceID())
	removedConfigs := cm.store.removeConfigsForService(svc.GetServiceID())
	for _, c := range removedConfigs {
		if cm.store.removeLoadedConfig(c) {
			changes.unscheduleConfig(c)
		}
	}

	return changes
}

// processNewConfig implements configManager#processNewConfig.
func (cm *simpleConfigManager) processNewConfig(config integration.Config) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()
	changes := configChanges{}

	if config.IsTemplate() {
		// store the template in the cache in any case
		if err := cm.store.templateCache.set(config); err != nil {
			log.Errorf("Unable to store Check configuration in the cache: %s", err)
		}

		// try to resolve the template
		resolvedConfigs := cm.resolveTemplate(config)
		if resolvedConfigs.isEmpty() {
			e := fmt.Sprintf("Can't resolve the template for %s at this moment.", config.Name)
			errorStats.setResolveWarning(config.Name, e)
			log.Debug(e)
			return changes // empty result
		}

		return resolvedConfigs
	}

	// decrypt and store non-template config in AC as well
	config, err := decryptConfig(config)
	if err != nil {
		log.Errorf("Dropping conf for '%s': %s", config.Name, err.Error())
		return changes // empty result
	}
	changes.scheduleConfig(config)
	cm.store.setLoadedConfig(config)

	return changes
}

// processDelConfigs implements configManager#processDelConfigs.
func (cm *simpleConfigManager) processDelConfigs(configs []integration.Config) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()
	changes := configChanges{}

	for _, c := range configs {
		if c.IsTemplate() {
			// Remove the resolved configurations
			tplDigest := c.Digest()
			removedConfigs := cm.store.removeConfigsForTemplate(tplDigest)
			for _, rc := range removedConfigs {
				if cm.store.removeLoadedConfig(rc) {
					changes.unscheduleConfig(rc)
				}
			}

			// Remove template from the cache
			err := cm.store.templateCache.del(c)
			if err != nil {
				log.Debugf("Could not delete template: %v", err)
			}
		} else {
			cm.store.removeLoadedConfig(c)
			changes.unscheduleConfig(c)
		}
	}

	return changes
}

// mapOverLoadedConfigs implements configManager#mapOverLoadedConfigs.
func (cm *simpleConfigManager) mapOverLoadedConfigs(f func(map[string]integration.Config)) {
	cm.m.Lock()
	defer cm.m.Unlock()
	cm.store.mapOverLoadedConfigs(f)
}

// resolveTemplateForService resolves a template config for the given service
func (cm *simpleConfigManager) resolveTemplateForService(tpl integration.Config, svc listeners.Service) (integration.Config, error) {
	config, err := configresolver.Resolve(tpl, svc)
	if err != nil {
		newErr := fmt.Errorf("error resolving template %s for service %s: %v", tpl.Name, svc.GetServiceID(), err)
		errorStats.setResolveWarning(tpl.Name, newErr.Error())
		return tpl, log.Warn(newErr)
	}
	resolvedConfig, err := decryptConfig(config)
	if err != nil {
		newErr := fmt.Errorf("error decrypting secrets in config %s for service %s: %v", config.Name, svc.GetServiceID(), err)
		return config, log.Warn(newErr)
	}
	cm.store.setLoadedConfig(resolvedConfig)
	cm.store.addConfigForService(svc.GetServiceID(), resolvedConfig)
	cm.store.addConfigForTemplate(tpl.Digest(), resolvedConfig)
	errorStats.removeResolveWarnings(tpl.Name)
	return resolvedConfig, nil
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
func (cm *simpleConfigManager) resolveTemplate(tpl integration.Config) configChanges {
	// use a map to dedupe configurations
	resolvedSet := map[string]integration.Config{}

	// go through the AD identifiers provided by the template
	for _, id := range tpl.ADIdentifiers {
		// check out whether any service we know has this identifier
		serviceIds, found := cm.store.getServiceEntitiesForADID(id)
		if !found {
			s := fmt.Sprintf("No service found with this AD identifier: %s", id)
			errorStats.setResolveWarning(tpl.Name, s)
			log.Debugf(s)
			continue
		}

		for serviceID := range serviceIds {
			svc := cm.store.getServiceForEntity(serviceID)
			if svc == nil {
				log.Warnf("Service %s was removed before we could resolve its config", serviceID)
				continue
			}
			resolvedConfig, err := cm.resolveTemplateForService(tpl, svc)
			if err != nil {
				continue
			}
			resolvedSet[resolvedConfig.Digest()] = resolvedConfig
		}
	}

	// build the config changes to return
	changes := configChanges{}
	for _, v := range resolvedSet {
		changes.scheduleConfig(v)
	}

	return changes
}

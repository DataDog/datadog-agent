// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"context"
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
	processDelService(ctx context.Context, svc listeners.Service) configChanges

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

// serviceAndADIDs bundles a service and its associated AD identifiers.
type serviceAndADIDs struct {
	svc   listeners.Service
	adIDs []string
}

// reconcilingConfigManager implements the a config manager that reconciles
// services and templates to generate the scheduled configs.
type reconcilingConfigManager struct {
	// updates to this data structure work from the top down:
	//
	//  1. update orctiveConfigs / activeServices
	//  2. update templatesByADID or servicesByADID to match
	//  3. update serviceResolutions, generating changes (see reconcileService)
	//  4. update scheduledConfigs
	//
	// For non-template configs, only steps 1 and 4 are required.

	// m synchronizes all operations on this struct.
	m sync.Mutex

	// activeConfigs contains an entry for each config from the config
	// providers, keyed by its digest.  This is the "base truth" of configs --
	// the set of new configs processed net deleted configs.
	activeConfigs map[string]integration.Config

	// activeServices contains an entry for each service from the listeners,
	// keyed by its serviceID and with its AD identifiers stored separately.
	// This is the "base truth" of services -- the set of new services
	// processed net deleted services.
	activeServices map[string]serviceAndADIDs

	// templatesByADID catalogs digests for all templates, indexed by their AD
	// identifiers.  It is an index to activeConfigs.
	templatesByADID multimap

	// servicesByADID catalogs serviceIDs for all services, indexed by their AD
	// identifiers.  It is an index to activeServices.
	servicesByADID multimap

	// serviceResolutions maps a serviceID to the resolutions performed for
	// that service: serviceID -> template digest -> resolved config digest.
	serviceResolutions map[string]map[string]string

	// scheduledConfigs contains an entry for each scheduled config, keyed by
	// its digest.  This is a mix of resolved templates and non-template
	// configs.  The returned configChanges from interface methods correspond
	// exactly to changes in this map.
	scheduledConfigs map[string]integration.Config
}

var _ configManager = &reconcilingConfigManager{}

// newReconcilingConfigManager creates a new, empty reconcilingConfigManager.
func newReconcilingConfigManager() configManager {
	return &reconcilingConfigManager{
		activeConfigs:      map[string]integration.Config{},
		activeServices:     map[string]serviceAndADIDs{},
		templatesByADID:    newMultimap(),
		servicesByADID:     newMultimap(),
		serviceResolutions: map[string]map[string]string{},
		scheduledConfigs:   map[string]integration.Config{},
	}
}

// processNewService implements configManager#processNewService.
func (cm *reconcilingConfigManager) processNewService(adIdentifiers []string, svc listeners.Service) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()

	svcID := svc.GetServiceID()
	if _, found := cm.activeServices[svcID]; found {
		log.Debugf("Service %s is already tracked by autodiscovery", svcID)
		return configChanges{}
	}

	// Execute the steps outlined in the comment on reconcilingConfigManager:
	//
	//  1. update orctiveConfigs / activeServices
	cm.activeServices[svcID] = serviceAndADIDs{
		svc:   svc,
		adIDs: adIdentifiers,
	}

	//  2. update templatesByADID or servicesByADID to match
	for _, adID := range adIdentifiers {
		cm.servicesByADID.insert(adID, svcID)
	}

	//  3. update serviceResolutions, generating changes
	changes := cm.reconcileService(svcID)

	//  4. update scheduledConfigs
	return cm.applyChanges(changes)
}

// processDelService implements configManager#processDelService.
func (cm *reconcilingConfigManager) processDelService(_ context.Context, svc listeners.Service) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()

	svcID := svc.GetServiceID()
	svcAndADIDs, found := cm.activeServices[svcID]
	if !found {
		log.Debugf("Service %s is not tracked by autodiscovery", svcID)
		return configChanges{}
	}

	// Execute the steps outlined in the comment on reconcilingConfigManager:
	//
	//  1. update activeConfigs or activeServices
	delete(cm.activeServices, svcID)

	//  2. update templatesByADID or servicesByADID to match
	for _, adID := range svcAndADIDs.adIDs {
		cm.servicesByADID.remove(adID, svcID)
	}

	//  3. update serviceResolutions, generating changes
	changes := cm.reconcileService(svcID)

	//  4. update scheduledConfigs
	return cm.applyChanges(changes)
}

// processNewConfig implements configManager#processNewConfig.
func (cm *reconcilingConfigManager) processNewConfig(config integration.Config) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()

	digest := config.Digest()
	if _, found := cm.activeConfigs[digest]; found {
		log.Debugf("Config %s (digest %s) is already tracked by autodiscovery", config.Name, config.Digest())
		return configChanges{}
	}

	// Execute the steps outlined in the comment on reconcilingConfigManager:
	//
	//  1. update orctiveConfigs / activeServices
	cm.activeConfigs[digest] = config

	var changes configChanges
	if config.IsTemplate() {
		//  2. update templatesByADID or servicesByADID to match
		matchingServices := map[string]struct{}{}
		for _, adID := range config.ADIdentifiers {
			cm.templatesByADID.insert(adID, digest)
			for _, svcID := range cm.servicesByADID.get(adID) {
				matchingServices[svcID] = struct{}{}
			}
		}

		//  3. update serviceResolutions, generating changes
		for svcID := range matchingServices {
			changes.merge(cm.reconcileService(svcID))
		}
	} else {
		changes.scheduleConfig(config)
	}

	//  4. update scheduledConfigs
	return cm.applyChanges(changes)
}

// processDelConfigs implements configManager#processDelConfigs.
func (cm *reconcilingConfigManager) processDelConfigs(configs []integration.Config) configChanges {
	cm.m.Lock()
	defer cm.m.Unlock()

	var allChanges configChanges
	for _, config := range configs {
		digest := config.Digest()
		if _, found := cm.activeConfigs[digest]; !found {
			log.Debug("Config %v is not tracked by autodiscovery", config.Name)
			continue
		}

		// Execute the steps outlined in the comment on reconcilingConfigManager:
		//
		//  1. update activeConfigs / activeServices
		delete(cm.activeConfigs, digest)

		var changes configChanges
		if config.IsTemplate() {
			//  2. update templatesByADID or servicesByADID to match
			matchingServices := map[string]struct{}{}
			for _, adID := range config.ADIdentifiers {
				cm.templatesByADID.remove(adID, digest)
				for _, svcID := range cm.servicesByADID.get(adID) {
					matchingServices[svcID] = struct{}{}
				}
			}

			//  3. update serviceResolutions, generating changes
			for svcID := range matchingServices {
				changes.merge(cm.reconcileService(svcID))
			}
		} else {
			changes.unscheduleConfig(config)
		}

		//  4. update scheduledConfigs
		allChanges.merge(cm.applyChanges(changes))
	}

	return allChanges
}

// mapOverLoadedConfigs implements configManager#mapOverLoadedConfigs.
func (cm *reconcilingConfigManager) mapOverLoadedConfigs(f func(map[string]integration.Config)) {
	cm.m.Lock()
	defer cm.m.Unlock()
	f(cm.scheduledConfigs)
}

// reconcileService calculates the current set of resolved templates for the
// given service and calculates the difference from what is currently recorded
// in cm.serviceResolutions.  It updates cm.serviceResolutions and returns the
// changes.
//
// This method must be called with cm.m locked.
func (cm *reconcilingConfigManager) reconcileService(svcID string) configChanges {
	var changes configChanges

	// note that this method can be called in a case where svcID is not in the
	// activeServices: this occurs when the service is removed.
	serviceAndADIDs := cm.activeServices[svcID]
	adIDs := serviceAndADIDs.adIDs // nil slice if service is not defined
	svc := serviceAndADIDs.svc     // nil if the service is not defined

	// get the existing resolutions for this service
	existingResolutions, found := cm.serviceResolutions[svcID]
	if !found {
		existingResolutions = map[string]string{}
	}

	// determine the matching templates by template digest.  If the service
	// has been removed, then this slice is empty.
	expectedResolutions := map[string]integration.Config{}
	for _, adID := range adIDs {
		digests := cm.templatesByADID.get(adID)
		for _, digest := range digests {
			tpl := cm.activeConfigs[digest]
			expectedResolutions[digest] = tpl
		}
	}

	// allow the service to filter those templates, unless we are removing
	// the service, in which case no resolutions are expected.
	if svc != nil {
		svc.FilterTemplates(expectedResolutions)
	}

	// compare existing to expected, generating changes and modifying
	// existingResolutions in-place
	for templateDigest, resolvedDigest := range existingResolutions {
		if _, found = expectedResolutions[templateDigest]; !found {
			changes.unscheduleConfig(cm.scheduledConfigs[resolvedDigest])
			delete(existingResolutions, templateDigest)
		}
	}

	for digest, config := range expectedResolutions {
		if _, found := existingResolutions[digest]; !found {
			// at this point, there was at least one expected resolution, so
			// svc must not be nil.
			resolved, ok := cm.resolveTemplateForService(config, svc)
			if !ok {
				continue
			}
			changes.scheduleConfig(resolved)
			existingResolutions[digest] = resolved.Digest()
		}
	}

	if len(existingResolutions) == 0 {
		delete(cm.serviceResolutions, svcID)
	} else {
		cm.serviceResolutions[svcID] = existingResolutions
	}

	return changes
}

// resolveTemplateForService resolves a template config for the given service,
// updating errorStats in the process.  If the resolution fails, this method
// returns false.
func (cm *reconcilingConfigManager) resolveTemplateForService(tpl integration.Config, svc listeners.Service) (integration.Config, bool) {
	config, err := configresolver.Resolve(tpl, svc)
	if err != nil {
		msg := fmt.Sprintf("error resolving template %s for service %s: %v", tpl.Name, svc.GetServiceID(), err)
		errorStats.setResolveWarning(tpl.Name, msg)
		return tpl, false
	}
	resolvedConfig, err := decryptConfig(config)
	if err != nil {
		msg := fmt.Sprintf("error decrypting secrets in config %s for service %s: %v", config.Name, svc.GetServiceID(), err)
		errorStats.setResolveWarning(tpl.Name, msg)
		return config, false
	}
	errorStats.removeResolveWarnings(tpl.Name)
	return resolvedConfig, true
}

// applyChanges applies the given changes to cm.scheduledConfigs
//
// This method must be called with cm.m locked.
func (cm *reconcilingConfigManager) applyChanges(changes configChanges) configChanges {
	for _, cfg := range changes.unschedule {
		digest := cfg.Digest()
		delete(cm.scheduledConfigs, digest)
	}
	for _, cfg := range changes.schedule {
		digest := cfg.Digest()
		cm.scheduledConfigs[digest] = cfg
	}

	return changes
}

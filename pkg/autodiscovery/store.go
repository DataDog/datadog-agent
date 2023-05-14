// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
)

// store holds useful mappings for the AD
type store struct {
	// serviceToConfigs maps service ID to a slice of resolved templates
	// for that service.  Configs are never removed from this map, even if the
	// template for which they were resolved is removed.
	serviceToConfigs map[string][]integration.Config

	// serviceToTagsHash maps tagger entity ID to a hash of the tags associated
	// with the service.  Note that this key differs from keys used elsewhere
	// in this type.
	serviceToTagsHash map[string]string

	// templateToConfigs maps config digest of a template to the resolved templates
	// created from it.  Configs are never removed from this map, even if the
	// service for which they were resolved is removed.
	templateToConfigs map[string][]integration.Config

	// loadedConfigs contains all scheduled configs (so, non-template configs
	// and resolved templates), indexed by their hash.
	loadedConfigs map[string]integration.Config

	// nameToJMXMetrics stores the MetricConfig for checks, keyed by check name.
	nameToJMXMetrics map[string]integration.Data

	// adIDToServices stores, for each AD identifier, the service IDs for
	// services with that AD identifier.  The map structure is
	// adIDTOServices[adID][serviceID] = struct{}{}
	adIDToServices map[string]map[string]struct{}

	// entityToService maps serviceIDs to Service instances.
	entityToService map[string]listeners.Service

	// templateCache stores templates by their AD identifiers.
	templateCache *templateCache

	// m is a Mutex protecting access to all fields in this type except
	// templateCache.
	m sync.RWMutex
}

// newStore creates a store
func newStore() *store {
	s := store{
		serviceToConfigs:  make(map[string][]integration.Config),
		serviceToTagsHash: make(map[string]string),
		templateToConfigs: make(map[string][]integration.Config),
		loadedConfigs:     make(map[string]integration.Config),
		nameToJMXMetrics:  make(map[string]integration.Data),
		adIDToServices:    make(map[string]map[string]struct{}),
		entityToService:   make(map[string]listeners.Service),
		templateCache:     newTemplateCache(),
	}

	return &s
}

// removeConfigsForService removes a config for a specified service, returning
// the configs that were removed
func (s *store) removeConfigsForService(serviceEntity string) []integration.Config {
	s.m.Lock()
	defer s.m.Unlock()
	removed := s.serviceToConfigs[serviceEntity]
	delete(s.serviceToConfigs, serviceEntity)
	return removed
}

// addConfigForService adds a config for a specified service
func (s *store) addConfigForService(serviceEntity string, config integration.Config) {
	s.m.Lock()
	defer s.m.Unlock()
	existingConfigs, found := s.serviceToConfigs[serviceEntity]
	if found {
		s.serviceToConfigs[serviceEntity] = append(existingConfigs, config)
	} else {
		s.serviceToConfigs[serviceEntity] = []integration.Config{config}
	}
}

// removeConfigsForTemplate removes all configs for a specified template, returning
// those configs
func (s *store) removeConfigsForTemplate(templateDigest string) []integration.Config {
	s.m.Lock()
	defer s.m.Unlock()
	removed := s.templateToConfigs[templateDigest]
	delete(s.templateToConfigs, templateDigest)
	return removed
}

// addConfigForTemplate adds a config for a specified template
func (s *store) addConfigForTemplate(templateDigest string, config integration.Config) {
	s.m.Lock()
	defer s.m.Unlock()
	s.templateToConfigs[templateDigest] = append(s.templateToConfigs[templateDigest], config)
}

// getTagsHashForService return the tags hash for a specified service
func (s *store) getTagsHashForService(serviceEntity string) string {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.serviceToTagsHash[serviceEntity]
}

// removeTagsHashForService removes the tags hash for a specified service
func (s *store) removeTagsHashForService(serviceEntity string) {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.serviceToTagsHash, serviceEntity)
}

// setTagsHashForService set the tags hash for a specified service
func (s *store) setTagsHashForService(serviceEntity string, hash string) {
	s.m.Lock()
	defer s.m.Unlock()
	s.serviceToTagsHash[serviceEntity] = hash
}

// setJMXMetricsForConfigName stores the jmx metrics config for a config name
func (s *store) setJMXMetricsForConfigName(config string, metrics integration.Data) {
	s.m.Lock()
	defer s.m.Unlock()
	s.nameToJMXMetrics[config] = metrics
}

// getJMXMetricsForConfigName returns the stored JMX metrics for a config name
func (s *store) getJMXMetricsForConfigName(config string) integration.Data {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.nameToJMXMetrics[config]
}

func (s *store) getServices() []listeners.Service {
	s.m.Lock()
	defer s.m.Unlock()
	services := []listeners.Service{}
	for _, service := range s.entityToService {
		services = append(services, service)
	}
	return services
}

func (s *store) setServiceForEntity(svc listeners.Service, entity string) {
	s.m.Lock()
	defer s.m.Unlock()
	s.entityToService[entity] = svc
}

func (s *store) removeServiceForEntity(entity string) {
	s.m.Lock()
	defer s.m.Unlock()
	delete(s.entityToService, entity)
}

func (s *store) setADIDForServices(adID string, serviceEntity string) {
	s.m.Lock()
	defer s.m.Unlock()
	if s.adIDToServices[adID] == nil {
		s.adIDToServices[adID] = make(map[string]struct{})
	}
	s.adIDToServices[adID][serviceEntity] = struct{}{}
}

func (s *store) getServiceEntitiesForADID(adID string) (map[string]struct{}, bool) {
	s.m.Lock()
	defer s.m.Unlock()
	services, found := s.adIDToServices[adID]
	return services, found
}

func (s *store) removeServiceForADID(entity string, adIdentifiers []string) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, adID := range adIdentifiers {
		if services, found := s.adIDToServices[adID]; found {
			delete(services, entity)
			if len(services) == 0 {
				// An AD identifier can be shared between multiple services (e.g image name)
				// We delete the AD identifier entry only when it doesn't match any existing service anymore.
				delete(s.adIDToServices, adID)
			}
		}
	}
}

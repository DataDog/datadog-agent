// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscovery

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/listeners"
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dispatcher holds the management logic for cluster-checks
type dispatcher struct {
	store *clusterStore
}

func newDispatcher() *dispatcher {
	return &dispatcher{
		store: newClusterStore(),
	}
}

// Stop implements the scheduler.Scheduler interface
// no-op for now
func (d *dispatcher) Stop() {
}

// Schedule implements the scheduler.Scheduler interface
func (d *dispatcher) Schedule(configs []integration.Config) {
	for _, c := range configs {
		d.add(c)
	}
}

// Unschedule implements the scheduler.Scheduler interface
func (d *dispatcher) Unschedule(configs []integration.Config) {
	for _, c := range configs {
		d.remove(c)
	}
}

// add stores and delegates a given configuration
func (d *dispatcher) add(config integration.Config) {
	if !config.ClusterCheck {
		return // Ignore non cluster-check configs
	}
	log.Debugf("dispatching configuration %s:%s", config.Name, config.Digest())

	// TODO: add dispatching logic
	hostname, _ := util.GetHostname()
	d.addConfig(config, hostname)
}

// remove deletes a given configuration
func (d *dispatcher) remove(config integration.Config) {
	if !config.ClusterCheck {
		return // Ignore non cluster-check configs
	}
	digest := config.Digest()
	log.Debugf("removing configuration %s:%s", config.Name, digest)
	d.removeConfig(digest)
}

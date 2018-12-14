// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dispatcher holds the management logic for cluster-checks
type dispatcher struct {
	store                 *clusterStore
	nodeExpirationSeconds int64
}

func newDispatcher() *dispatcher {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.nodeExpirationSeconds = config.Datadog.GetInt64("cluster_checks.node_expiration_timeout")
	return d
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

	target := d.getLeastBusyNode()
	if target == "" {
		// If no node is found, store it in the danglingConfigs map for retrying later.
		log.Warnf("No available node to dispatch %s:%s on, will retry later", config.Name, config.Digest())
	} else {
		log.Infof("Dispatching configuration %s:%s to node %s", config.Name, config.Digest(), target)
	}

	d.addConfig(config, target)
}

// remove deletes a given configuration
func (d *dispatcher) remove(config integration.Config) {
	if !config.ClusterCheck {
		return // Ignore non cluster-check configs
	}
	digest := config.Digest()
	log.Debugf("Removing configuration %s:%s", config.Name, digest)
	d.removeConfig(digest)
}

// cleanupLoop is the cleanup goroutine for the dispatcher.
// It has to be called in a goroutine with a cancellable context.
func (d *dispatcher) cleanupLoop(ctx context.Context) {
	healthProbe := health.Register("clusterchecks-dispatch")
	defer health.Deregister(healthProbe)

	cleanupTicker := time.NewTicker(time.Duration(d.nodeExpirationSeconds/2) * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-healthProbe.C:
			// This goroutine might hang if the store is deadlocked during a cleanup
		case <-cleanupTicker.C:
			// Expire old nodes, orphaned configs are moved to dangling
			d.expireNodes()

			// Re-dispatch dangling configs
			if d.shouldDispatchDanling() {
				d.Schedule(d.retrieveAndClearDangling())
			}
		}
	}
}

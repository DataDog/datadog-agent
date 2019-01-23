// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dispatcher holds the management logic for cluster-checks
type dispatcher struct {
	store                 *clusterStore
	nodeExpirationSeconds int64
	extraTags             []string
}

func newDispatcher() *dispatcher {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.nodeExpirationSeconds = config.Datadog.GetInt64("cluster_checks.node_expiration_timeout")

	clusterTagValue := clustername.GetClusterName()
	clusterTagName := config.Datadog.GetString("cluster_checks.cluster_tag_name")
	if clusterTagName != "" && clusterTagValue != "" {
		d.extraTags = append(d.extraTags, fmt.Sprintf("%s:%s", clusterTagName, clusterTagValue))
	}

	return d
}

// Stop implements the scheduler.Scheduler interface
// no-op for now
func (d *dispatcher) Stop() {
}

// Schedule implements the scheduler.Scheduler interface
func (d *dispatcher) Schedule(configs []integration.Config) {
	for _, c := range configs {
		if !c.ClusterCheck {
			continue // Ignore non cluster-check configs
		}
		patched, err := d.patchConfiguration(c)
		if err != nil {
			log.Warnf("Cannot patch configuration %s: %s", c.Digest(), err)
			continue
		}
		d.add(patched)
	}
}

// Unschedule implements the scheduler.Scheduler interface
func (d *dispatcher) Unschedule(configs []integration.Config) {
	for _, c := range configs {
		if !c.ClusterCheck {
			continue // Ignore non cluster-check configs
		}
		patched, err := d.patchConfiguration(c)
		if err != nil {
			log.Warnf("Cannot patch configuration %s: %s", c.Digest(), err)
			continue
		}
		d.remove(patched)
	}
}

// add stores and delegates a given configuration
func (d *dispatcher) add(config integration.Config) {
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
	digest := config.Digest()
	log.Debugf("Removing configuration %s:%s", config.Name, digest)
	d.removeConfig(digest)
}

// reset empties the store and resets all states
func (d *dispatcher) reset() {
	d.store.Lock()
	defer d.store.Unlock()
	d.store.reset()
}

// run is the main management goroutine for the dispatcher
func (d *dispatcher) run(ctx context.Context) {
	d.store.Lock()
	d.store.active = true
	d.store.Unlock()

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

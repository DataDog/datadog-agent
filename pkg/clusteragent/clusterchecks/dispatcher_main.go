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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ktypes "k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/client-go/listers/core/v1"
)

// dispatcher holds the management logic for cluster-checks
type dispatcher struct {
	store                 *clusterStore
	nodeExpirationSeconds int64
	extraTags             []string
	endpointsLister       v1.EndpointsLister
}

func newDispatcher() *dispatcher {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.nodeExpirationSeconds = config.Datadog.GetInt64("cluster_checks.node_expiration_timeout")
	d.extraTags = config.Datadog.GetStringSlice("cluster_checks.extra_tags")

	clusterTagValue := clustername.GetClusterName()
	clusterTagName := config.Datadog.GetString("cluster_checks.cluster_tag_name")
	if clusterTagName != "" && clusterTagValue != "" {
		d.extraTags = append(d.extraTags, fmt.Sprintf("%s:%s", clusterTagName, clusterTagValue))
	}
	var err error
	d.endpointsLister, err = newEndpointsLister()
	if err != nil {
		log.Errorf("Cannot create endpoints lister: %s", err)
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
		if isKubeServiceCheck(c) && len(c.EndpointsChecks) > 0 {
			// A kube service that requires endpoints checks will be scheduled,
			// endpoints cache must be updated with the new checks.
			d.store.Lock()
			d.store.endpointsCache[ktypes.UID(getServiceUID(c))] = newEndpointsInfo(c)
			d.store.Unlock()
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
		if isKubeServiceCheck(c) && len(c.EndpointsChecks) > 0 {
			d.store.Lock()
			// Remove the cached endpoints checks of the service
			delete(d.store.endpointsCache, ktypes.UID(getServiceUID(c)))
			d.store.Unlock()
		}
		patched, err := d.patchConfiguration(c)
		if err != nil {
			log.Warnf("Cannot patch configuration %s: %s", c.Digest(), err)
			continue
		}
		d.remove(patched)
	}
}

// reschdule sends configurations to dispatching without checking or patching them as Schedule does.
func (d *dispatcher) reschedule(configs []integration.Config) {
	for _, c := range configs {
		log.Debugf("Rescheduling the check %s:%s", c.Name, c.Digest())
		d.add(c)
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

	registerMetrics()
	defer unregisterMetrics()

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
				danglingConfs := d.retrieveAndClearDangling()
				d.reschedule(danglingConfs)
			}
		}
	}
}

// newEndpointsInfo initializes an EndpointsInfo struct from a service config.
// Needed by the endpoints configs dispatching logic to validate the checks.
func newEndpointsInfo(c integration.Config) *types.EndpointsInfo {
	var namespace string
	var name string
	if len(c.EndpointsChecks) > 0 {
		namespace, name = getNameAndNamespaceFromADIDs(c.EndpointsChecks)
	}
	return &types.EndpointsInfo{
		ServiceEntity: c.Entity,
		Namespace:     namespace,
		Name:          name,
		Configs:       c.EndpointsChecks,
	}
}

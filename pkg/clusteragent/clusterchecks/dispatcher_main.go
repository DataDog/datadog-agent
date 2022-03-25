// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks
// +build clusterchecks

package clusterchecks

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const firstRunnerStatsMinutes = 2  // collect runner stats after the first 2 minutes
const secondRunnerStatsMinutes = 5 // collect runner stats after the first 7 minutes
const finalRunnerStatsMinutes = 10 // collect runner stats endlessly every 10 minutes

// dispatcher holds the management logic for cluster-checks
type dispatcher struct {
	store                 *clusterStore
	nodeExpirationSeconds int64
	extraTags             []string
	clcRunnersClient      clusteragent.CLCRunnerClientInterface
	advancedDispatching   bool
}

func newDispatcher() *dispatcher {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.nodeExpirationSeconds = config.Datadog.GetInt64("cluster_checks.node_expiration_timeout")
	d.extraTags = config.Datadog.GetStringSlice("cluster_checks.extra_tags")

	hname, _ := hostname.Get(context.TODO())
	clusterTagValue := clustername.GetClusterName(context.TODO(), hname)
	clusterTagName := config.Datadog.GetString("cluster_checks.cluster_tag_name")
	if clusterTagValue != "" {
		if clusterTagName != "" && !config.Datadog.GetBool("disable_cluster_name_tag_key") {
			d.extraTags = append(d.extraTags, fmt.Sprintf("%s:%s", clusterTagName, clusterTagValue))
			log.Info("Adding both tags cluster_name and kube_cluster_name. You can use 'disable_cluster_name_tag_key' in the Agent config to keep the kube_cluster_name tag only")
		}
		d.extraTags = append(d.extraTags, fmt.Sprintf("kube_cluster_name:%s", clusterTagValue))
	}

	d.advancedDispatching = config.Datadog.GetBool("cluster_checks.advanced_dispatching_enabled")
	if !d.advancedDispatching {
		return d
	}

	var err error
	d.clcRunnersClient, err = clusteragent.GetCLCRunnerClient()
	if err != nil {
		log.Warnf("Cannot create CLC runners client, advanced dispatching will be disabled: %v", err)
		d.advancedDispatching = false
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
		if c.NodeName != "" {
			// An endpoint check backed by a pod
			patched, err := d.patchEndpointsConfiguration(c)
			if err != nil {
				log.Warnf("Cannot patch endpoint configuration %s: %s", c.Digest(), err)
				continue
			}
			d.addEndpointConfig(patched, c.NodeName)
			continue
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
		if c.NodeName != "" {
			patched, err := d.patchEndpointsConfiguration(c)
			if err != nil {
				log.Warnf("Cannot patch endpoint configuration %s: %s", c.Digest(), err)
				continue
			}
			d.removeEndpointConfig(patched, c.NodeName)
			continue
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

	healthProbe := health.RegisterLiveness("clusterchecks-dispatch")
	defer health.Deregister(healthProbe) //nolint:errcheck

	cleanupTicker := time.NewTicker(time.Duration(d.nodeExpirationSeconds/2) * time.Second)
	defer cleanupTicker.Stop()

	runnerStatsMinutes := firstRunnerStatsMinutes
	runnerStatsTicker := time.NewTicker(time.Duration(runnerStatsMinutes) * time.Minute)
	defer runnerStatsTicker.Stop()

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
		case <-runnerStatsTicker.C:
			// Collect stats with an exponential backoff 2 - 5 - 10 minutes
			if runnerStatsMinutes == firstRunnerStatsMinutes {
				runnerStatsMinutes = secondRunnerStatsMinutes
				runnerStatsTicker = time.NewTicker(time.Duration(runnerStatsMinutes) * time.Minute)
			} else if runnerStatsMinutes == secondRunnerStatsMinutes {
				runnerStatsMinutes = finalRunnerStatsMinutes
				runnerStatsTicker = time.NewTicker(time.Duration(runnerStatsMinutes) * time.Minute)
			}

			// Rebalance if needed
			if d.advancedDispatching {
				// Rebalance checks distribution
				d.rebalance()
			}
		}
	}
}

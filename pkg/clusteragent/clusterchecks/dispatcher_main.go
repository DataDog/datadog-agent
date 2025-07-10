// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// dispatcher holds the management logic for cluster-checks
type dispatcher struct {
	store                            *clusterStore
	nodeExpirationSeconds            int64
	unscheduledCheckThresholdSeconds int64
	extraTags                        []string
	clcRunnersClient                 clusteragent.CLCRunnerClientInterface
	advancedDispatching              bool
	excludedChecks                   map[string]struct{}
	excludedChecksFromDispatching    map[string]struct{}
	rebalancingPeriod                time.Duration
}

func newDispatcher(tagger tagger.Component) *dispatcher {
	d := &dispatcher{
		store: newClusterStore(),
	}
	d.nodeExpirationSeconds = pkgconfigsetup.Datadog().GetInt64("cluster_checks.node_expiration_timeout")
	d.unscheduledCheckThresholdSeconds = pkgconfigsetup.Datadog().GetInt64("cluster_checks.unscheduled_check_threshold")

	if d.unscheduledCheckThresholdSeconds < d.nodeExpirationSeconds {
		log.Warnf("The unscheduled_check_threshold value should be larger than node_expiration_timeout, setting it to the same value")
		d.unscheduledCheckThresholdSeconds = d.nodeExpirationSeconds
	}

	// Attach the cluster agent's global tags to all dispatched checks
	// as defined in the tagger's workloadmeta collector
	var err error
	d.extraTags, err = tagger.GlobalTags(types.LowCardinality)
	if err != nil {
		log.Warnf("Cannot get global tags from the tagger: %v", err)
	} else {
		log.Debugf("Adding global tags to cluster check dispatcher: %v", d.extraTags)
	}

	excludedChecks := pkgconfigsetup.Datadog().GetStringSlice("cluster_checks.exclude_checks")
	// This option will almost always be empty
	if len(excludedChecks) > 0 {
		d.excludedChecks = make(map[string]struct{}, len(excludedChecks))
		for _, checkName := range excludedChecks {
			d.excludedChecks[checkName] = struct{}{}
		}
	}

	excludedChecksFromDispatching := pkgconfigsetup.Datadog().GetStringSlice("cluster_checks.exclude_checks_from_dispatching")
	// This option will almost always be empty
	if len(excludedChecksFromDispatching) > 0 {
		d.excludedChecksFromDispatching = make(map[string]struct{}, len(excludedChecksFromDispatching))
		for _, checkName := range excludedChecksFromDispatching {
			d.excludedChecksFromDispatching[checkName] = struct{}{}
		}
	}

	d.rebalancingPeriod = pkgconfigsetup.Datadog().GetDuration("cluster_checks.rebalance_period")

	hname, _ := hostname.Get(context.TODO())
	clusterTagValue := clustername.GetClusterName(context.TODO(), hname)
	clusterTagName := pkgconfigsetup.Datadog().GetString("cluster_checks.cluster_tag_name")
	if clusterTagValue != "" {
		if clusterTagName != "" && !pkgconfigsetup.Datadog().GetBool("disable_cluster_name_tag_key") {
			d.extraTags = append(d.extraTags, fmt.Sprintf("%s:%s", clusterTagName, clusterTagValue))
			log.Info("Adding both tags cluster_name and kube_cluster_name. You can use 'disable_cluster_name_tag_key' in the Agent config to keep the kube_cluster_name tag only")
		}
		d.extraTags = append(d.extraTags, tags.KubeClusterName+":"+clusterTagValue)
	}

	clusterIDTagValue, _ := clustername.GetClusterID()
	if clusterIDTagValue != "" {
		d.extraTags = append(d.extraTags, tags.OrchClusterID+":"+clusterIDTagValue)
	}

	d.advancedDispatching = pkgconfigsetup.Datadog().GetBool("cluster_checks.advanced_dispatching_enabled")
	if !d.advancedDispatching {
		return d
	}

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
		if _, found := d.excludedChecks[c.Name]; found {
			log.Infof("Excluding check due to config: %s", c.Name)
			continue
		}

		if !c.ClusterCheck {
			continue // Ignore non cluster-check configs
		}

		if c.HasFilter(workloadfilter.MetricsFilter) || c.HasFilter(workloadfilter.GlobalFilter) {
			log.Debugf("Config %s is filtered out for metrics collection, ignoring it", c.Name)
			continue
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
func (d *dispatcher) reschedule(configs []integration.Config) []string {
	addedConfigIDs := make([]string, 0, len(configs))
	for _, c := range configs {
		log.Debugf("Rescheduling the check %s:%s", c.Name, c.Digest())
		if d.add(c) {
			addedConfigIDs = append(addedConfigIDs, c.Digest())
		}
	}
	return addedConfigIDs
}

// add stores and delegates a given configuration
func (d *dispatcher) add(config integration.Config) bool {
	target := d.getNodeToScheduleCheck()
	if target == "" {
		// If no node is found, store it in the danglingConfigs map for retrying later.
		log.Warnf("No available node to dispatch %s:%s on, will retry later", config.Name, config.Digest())
	} else {
		log.Infof("Dispatching configuration %s:%s to node %s", config.Name, config.Digest(), target)
	}

	return d.addConfig(config, target)
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

// scanUnscheduledChecks scans the store for configs that have been
// unscheduled for longer than the unscheduledCheckThresholdSeconds
func (d *dispatcher) scanUnscheduledChecks() {
	d.store.Lock()
	defer d.store.Unlock()

	for _, c := range d.store.danglingConfigs {
		if !c.unscheduledCheck && c.isStuckScheduling(d.unscheduledCheckThresholdSeconds) {
			log.Warnf("Detected unscheduled check config. Name:%s, Source:%s", c.config.Name, c.config.Source)
			c.unscheduledCheck = true
			unscheduledCheck.Inc(le.JoinLeaderValue, c.config.Name, c.config.Source)
		}
	}
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

	rebalanceTicker := time.NewTicker(d.rebalancingPeriod)
	defer rebalanceTicker.Stop()

	unscheduledCheckTicker := time.NewTicker(time.Duration(d.unscheduledCheckThresholdSeconds) * time.Second)
	defer unscheduledCheckTicker.Stop()

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
			if d.shouldDispatchDangling() {
				danglingConfigs := d.retrieveDangling()
				scheduledConfigIDs := d.reschedule(danglingConfigs)
				d.store.Lock()
				d.deleteDangling(scheduledConfigIDs)
				d.store.Unlock()
			}
		case <-unscheduledCheckTicker.C:
			// Check for configs that have been dangling longer than expected
			d.scanUnscheduledChecks()
		case <-rebalanceTicker.C:
			if d.advancedDispatching {
				d.rebalance(false)
			}
		}
	}
}

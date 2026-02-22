// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	cctypes "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
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
	advancedDispatching              atomic.Bool
	excludedChecks                   map[string]struct{}
	excludedChecksFromDispatching    map[string]struct{}
	rebalancingPeriod                time.Duration
	ksmSharding                      *ksmShardingManager
	ksmShardingMutex                 sync.Mutex          // Protects ksmShardedConfigs
	ksmShardedConfigs                map[string][]string // Maps original config digest -> shard digests, protected by ksmShardingMutex
	tracingEnabled                   bool
}

func newDispatcher(tagger tagger.Component) *dispatcher {
	d := &dispatcher{
		store:             newClusterStore(),
		ksmShardedConfigs: make(map[string][]string),
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

	hname, _ := hostname.Get(context.TODO())
	clusterTagValue := clustername.GetClusterName(context.TODO(), hname)
	clusterTagName := pkgconfigsetup.Datadog().GetString("cluster_checks.cluster_tag_name")
	if clusterTagValue != "" {
		if clusterTagName != "" && !pkgconfigsetup.Datadog().GetBool("disable_cluster_name_tag_key") {
			d.extraTags = append(d.extraTags, clusterTagName+":"+clusterTagValue)
			log.Info("Adding both tags cluster_name and kube_cluster_name. You can use 'disable_cluster_name_tag_key' in the Agent config to keep the kube_cluster_name tag only")
		}
		d.extraTags = append(d.extraTags, tags.KubeClusterName+":"+clusterTagValue)
	}

	clusterIDTagValue, err := clustername.GetClusterID()
	if err != nil {
		log.Warnf("Failed to get cluster ID: %v", err)
	}
	if clusterIDTagValue != "" {
		d.extraTags = append(d.extraTags, tags.OrchClusterID+":"+clusterIDTagValue)
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

	d.tracingEnabled = pkgconfigsetup.Datadog().GetBool("cluster_agent.tracing.enabled")
	d.rebalancingPeriod = pkgconfigsetup.Datadog().GetDuration("cluster_checks.rebalance_period")
	advancedDispatchingEnabled := pkgconfigsetup.Datadog().GetBool("cluster_checks.advanced_dispatching_enabled")
	if !advancedDispatchingEnabled {
		return d
	}

	d.clcRunnersClient, err = clusteragent.GetCLCRunnerClient()
	if err != nil {
		log.Warnf("Cannot create CLC runners client, advanced dispatching will be disabled: %v", err)
	} else {
		d.advancedDispatching.Store(true)
	}

	// Initialize KSM sharding (requires advanced dispatching)
	// Advanced dispatching is required for KSM sharding because it ensures shards are distributed across runners
	ksmShardingEnabled := pkgconfigsetup.Datadog().GetBool("cluster_checks.ksm_sharding_enabled")
	if ksmShardingEnabled {
		// Validate advanced dispatching is actually enabled
		if !d.advancedDispatching.Load() {
			log.Warn("KSM resource sharding requires advanced dispatching (cluster_checks.advanced_dispatching_enabled=true). Disabling KSM sharding.")
			ksmShardingEnabled = false
		} else {
			// KSM sharding configuration notes:
			// - Namespace labels/annotations as tags require GLOBAL config (kubernetes_resources_labels_as_tags)
			// - Check-specific labels_as_tags in KSM config is NOT supported with sharding
			// - Sharding also breaks check-specific label_joins across different resource types
			log.Info("KSM resource sharding enabled. For namespace labels/annotations as tags, check-specific config (labels_as_tags in KSM config) is not supported with sharding - use global kubernetes_resources_labels_as_tags instead.")
		}
	}

	d.ksmSharding = newKSMShardingManager(ksmShardingEnabled)

	return d
}

// Stop implements the scheduler.Scheduler interface
// no-op for now
func (d *dispatcher) Stop() {
}

// Schedule implements the scheduler.Scheduler interface
func (d *dispatcher) Schedule(configs []integration.Config) {
	var failedConfigs, excludedConfigs int
	if d.tracingEnabled {
		span := tracer.StartSpan("cluster_checks.dispatcher.schedule",
			tracer.ResourceName("schedule_configs"),
			tracer.SpanType("worker"))
		span.SetTag("config_count", len(configs))
		checkNames := make([]string, 0, len(configs))
		for _, c := range configs {
			checkNames = append(checkNames, c.Name)
		}
		span.SetTag("check_names", strings.Join(checkNames, ","))
		defer func() {
			span.SetTag("excluded_configs", excludedConfigs)
			span.SetTag("failed_configs", failedConfigs)
			if failedConfigs > 0 {
				span.SetTag("error", true)
			}
			span.Finish()
		}()
	}

	for _, c := range configs {
		if _, found := d.excludedChecks[c.Name]; found {
			log.Infof("Excluding check due to config: %s", c.Name)
			excludedConfigs++
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
				failedConfigs++
				continue
			}
			d.addEndpointConfig(patched, c.NodeName)
			continue
		}

		// Try to handle KSM sharding
		if d.scheduleKSMCheck(c) {
			// KSM check was sharded and scheduled, skip normal scheduling
			continue
		}

		patched, err := d.patchConfiguration(c)
		if err != nil {
			log.Warnf("Cannot patch configuration %s: %s", c.Digest(), err)
			failedConfigs++
			continue
		}
		d.add(patched)
	}
}

// Unschedule implements the scheduler.Scheduler interface
func (d *dispatcher) Unschedule(configs []integration.Config) {
	var failedConfigs int
	if d.tracingEnabled {
		span := tracer.StartSpan("cluster_checks.dispatcher.unschedule",
			tracer.ResourceName("unschedule_configs"),
			tracer.SpanType("worker"))
		span.SetTag("config_count", len(configs))
		defer func() {
			span.SetTag("failed_configs", failedConfigs)
			if failedConfigs > 0 {
				span.SetTag("error", true)
			}
			span.Finish()
		}()
	}

	for _, c := range configs {
		if !c.ClusterCheck {
			continue // Ignore non cluster-check configs
		}

		// Check if this is a sharded KSM check and remove all shards
		if d.unscheduleKSMCheck(c) {
			continue // Sharded configs were removed, skip normal unscheduling
		}

		if c.NodeName != "" {
			patched, err := d.patchEndpointsConfiguration(c)
			if err != nil {
				log.Warnf("Cannot patch endpoint configuration %s: %s", c.Digest(), err)
				failedConfigs++
				continue
			}
			d.removeEndpointConfig(patched, c.NodeName)
			continue
		}
		patched, err := d.patchConfiguration(c)
		if err != nil {
			log.Warnf("Cannot patch configuration %s: %s", c.Digest(), err)
			failedConfigs++
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

// UpdateAdvancedDispatchingMode checks if any node agents are in the pool
// and disables advanced dispatching if found
func (d *dispatcher) UpdateAdvancedDispatchingMode() {
	if !d.advancedDispatching.Load() {
		return
	}

	d.store.RLock()
	defer d.store.RUnlock()

	// Check if any node agents are in the pool
	hasNodeAgent := false
	for _, node := range d.store.nodes {
		if node.nodetype == cctypes.NodeTypeNodeAgent {
			hasNodeAgent = true
			break
		}
	}

	if hasNodeAgent {
		d.disableAdvancedDispatching()
	}
}

// disableAdvancedDispatching disables advanced dispatching mode
func (d *dispatcher) disableAdvancedDispatching() {
	if d.advancedDispatching.CompareAndSwap(true, false) {
		log.Info("Node agents detected in cluster check pool, disabling advanced dispatching")
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
			if d.advancedDispatching.Load() {
				d.rebalance(false)
			}
		}
	}
}

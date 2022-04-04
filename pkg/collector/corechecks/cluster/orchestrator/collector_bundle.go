// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package orchestrator

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/tools/cache"
)

const (
	defaultExtraSyncTimeout = 60 * time.Second
)

// CollectorBundle is a container for a group of collectors. It provides a way
// to easily run them all.
type CollectorBundle struct {
	check              *OrchestratorCheck
	collectors         []collectors.Collector
	discoverCollectors bool
	extraSyncTimeout   time.Duration
	inventory          *inventory.CollectorInventory
	stopCh             chan struct{}
	runCfg             *collectors.CollectorRunConfig
}

// NewCollectorBundle creates a new bundle from the check configuration.
//
// If collectors are declared in the check instance configuration then it'll
// only select those. This needs to match what is found in
// https://github.com/kubernetes/kube-state-metrics/blob/09539977815728349522b58154d800e4b517ec9c/internal/store/builder.go#L176-L206
// in order to share/split easily the collector configuration with the KSM core
// check.
//
// If that's not the case then it'll select all available collectors that are
// marked as stable.
func NewCollectorBundle(chk *OrchestratorCheck) *CollectorBundle {
	bundle := &CollectorBundle{
		discoverCollectors: chk.orchestratorConfig.CollectorDiscoveryEnabled,
		check:              chk,
		inventory:          inventory.NewCollectorInventory(),
		runCfg: &collectors.CollectorRunConfig{
			APIClient:   chk.apiClient,
			ClusterID:   chk.clusterID,
			Config:      chk.orchestratorConfig,
			MsgGroupRef: chk.groupID,
		},
		stopCh: make(chan struct{}),
	}

	bundle.prepare()

	return bundle
}

// prepare initializes the collector bundle internals before it can be used.
func (cb *CollectorBundle) prepare() {
	cb.prepareCollectors()
	cb.prepareExtraSyncTimeout()
}

// prepareCollectors initializes the bundle collector list.
func (cb *CollectorBundle) prepareCollectors() {
	// Custom collector list provided in the check configuration.
	if len(cb.check.instance.Collectors) > 0 {
		for _, c := range cb.check.instance.Collectors {
			cb.addCollectorFromConfig(c)
		}
		return
	}

	// Discover collectors from the list of resources exposed by the API server.
	if cb.discoverCollectors {
		provider := NewAPIServerDiscoveryProvider()

		if collectors, err := provider.Discover(cb.inventory); err != nil {
			_ = cb.check.Warnf("Collector discovery failed: %s", err)
		} else if len(cb.collectors) > 0 {
			cb.collectors = append(cb.collectors, collectors...)
			return
		}
	}

	// Finally, fall back to the list of stable collectors with default
	// versions.
	cb.collectors = cb.inventory.StableCollectors()
	return
}

// addCollectorFromConfig appends a collector to the bundle based on the
// collector name specified in the check configuration.
//
// The following configuration keys are accepted:
//   - <collector_name> (e.g "cronjobs")
//   - <apigroup_and_version>/<collector_name> (e.g. "batch/v1/cronjobs")
//
// Note that in the versionless case the collector version that'll be used is
// the one declared as the default version in the inventory.
func (cb *CollectorBundle) addCollectorFromConfig(collectorName string) {
	var (
		collector collectors.Collector
		err       error
	)

	if idx := strings.LastIndex(collectorName, "/"); idx != -1 {
		version := collectorName[:idx]
		name := collectorName[idx+1:]
		collector, err = cb.inventory.CollectorForVersion(name, version)
	} else {
		collector, err = cb.inventory.CollectorForDefaultVersion(collectorName)
	}

	if err != nil {
		_ = cb.check.Warnf("Unsupported collector: %s", collectorName)
		return
	}

	if !collector.Metadata().IsStable {
		_ = cb.check.Warnf("Using unstable collector: %s", collector.Metadata().FullName())
	}

	cb.collectors = append(cb.collectors, collector)
}

// prepareExtraSyncTimeout initializes the bundle extra sync timeout.
func (cb *CollectorBundle) prepareExtraSyncTimeout() {
	// No extra timeout set in the check configuration.
	// Use the default.
	if cb.check.instance.ExtraSyncTimeoutSeconds <= 0 {
		cb.extraSyncTimeout = defaultExtraSyncTimeout
		return
	}

	// Custom extra timeout.
	cb.extraSyncTimeout = time.Duration(cb.check.instance.ExtraSyncTimeoutSeconds) * time.Second
}

// Initialize is used to initialize collectors part of the bundle.
// During initialization informers are created, started and their cache is
// synced.
func (cb *CollectorBundle) Initialize() error {
	informersToSync := make(map[apiserver.InformerName]cache.SharedInformer)
	var availableCollectors []collectors.Collector
	// informerSynced is a helper map which makes sure that we don't initialize the same informer twice.
	// i.e. the cluster and nodes resources share the same informer and using both can lead to a race condition activating both concurrently.
	informerSynced := map[cache.SharedInformer]struct{}{}

	for _, collector := range cb.collectors {
		collector.Init(cb.runCfg)
		if !collector.IsAvailable() {
			_ = cb.check.Warnf("Collector %q is unavailable, skipping it", collector.Metadata().FullName())
			continue
		}

		availableCollectors = append(availableCollectors, collector)

		informer := collector.Informer()

		if _, found := informerSynced[informer]; !found {
			informersToSync[apiserver.InformerName(collector.Metadata().FullName())] = informer
			informerSynced[informer] = struct{}{}
			// we run each enabled informer individually, because starting them through the factory
			// would prevent us from restarting them again if the check is unscheduled/rescheduled
			// see https://github.com/kubernetes/client-go/blob/3511ef41b1fbe1152ef5cab2c0b950dfd607eea7/informers/factory.go#L64-L66

			// TODO: right now we use a stop channel which we don't close, that can lead to resource leaks
			// A recent go-client update https://github.com/kubernetes/kubernetes/pull/104853 changed the behaviour so that
			// we are not able to start informers anymore once they have been stopped. We will need to work around this. Once this is fixed we can properly release the resources during a check.Close().
			go informer.Run(cb.stopCh)
		}
	}

	cb.collectors = availableCollectors

	return apiserver.SyncInformers(informersToSync, cb.extraSyncTimeout)
}

// Run is used to sequentially run all collectors in the bundle.
func (cb *CollectorBundle) Run(sender aggregator.Sender) {
	for _, collector := range cb.collectors {
		runStartTime := time.Now()

		result, err := collector.Run(cb.runCfg)
		if err != nil {
			_ = cb.check.Warnf("Collector %s failed to run: %s", collector.Metadata().FullName(), err.Error())
			continue
		}

		runDuration := time.Since(runStartTime)
		log.Debugf("Collector %s run stats: listed=%d processed=%d messages=%d duration=%s", collector.Metadata().FullName(), result.ResourcesListed, result.ResourcesProcessed, len(result.Result.MetadataMessages), runDuration)

		orchestrator.SetCacheStats(result.ResourcesListed, len(result.Result.MetadataMessages), collector.Metadata().NodeType)
		sender.OrchestratorMetadata(result.Result.MetadataMessages, cb.check.clusterID, int(collector.Metadata().NodeType))
		if cb.runCfg.Config.IsManifestCollectionEnabled {
			sender.OrchestratorManifest(result.Result.ManifestMessages, cb.check.clusterID)
		}
	}
}

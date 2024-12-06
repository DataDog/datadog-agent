// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	"expvar"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/tools/cache"
)

const (
	defaultExtraSyncTimeout = 60 * time.Second
	defaultMaximumCRDs      = 100
)

var (
	skippedResourcesExpVars = expvar.NewMap("orchestrator-skipped-resources")
	skippedResources        = map[string]*expvar.String{}

	tlmSkippedResources = telemetry.NewCounter("orchestrator", "skipped_resources", []string{"name"}, "Skipped resources in orchestrator check")
)

// CollectorBundle is a container for a group of collectors. It provides a way
// to easily run them all.
type CollectorBundle struct {
	check               *OrchestratorCheck
	collectors          []collectors.K8sCollector
	discoverCollectors  bool
	extraSyncTimeout    time.Duration
	inventory           *inventory.CollectorInventory
	stopCh              chan struct{}
	runCfg              *collectors.CollectorRunConfig
	manifestBuffer      *ManifestBuffer
	collectorDiscovery  *discovery.DiscoveryCollector
	activatedCollectors map[string]struct{}
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
		inventory:          inventory.NewCollectorInventory(chk.cfg, chk.wlmStore, chk.tagger),
		runCfg: &collectors.CollectorRunConfig{
			K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
				APIClient:                   chk.apiClient,
				OrchestratorInformerFactory: chk.orchestratorInformerFactory,
			},
			ClusterID:   chk.clusterID,
			Config:      chk.orchestratorConfig,
			MsgGroupRef: chk.groupID,
		},
		stopCh:              chk.stopCh,
		manifestBuffer:      NewManifestBuffer(chk),
		collectorDiscovery:  discovery.NewDiscoveryCollectorForInventory(),
		activatedCollectors: map[string]struct{}{},
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
	// we still need to collect non crd resources except if otherwise configured
	if ok := cb.importCRDCollectorsFromCheckConfig(); ok {
		if cb.skipImportingDefaultCollectors() {
			return
		}
	}

	if ok := cb.importCollectorsFromCheckConfig(); ok {
		return
	}
	if ok := cb.importCollectorsFromDiscovery(); ok {
		return
	}

	cb.importCollectorsFromInventory()
}

// skipImportingDefaultCollectors skips importing the default collectors if the collector list is explicitly set to an
// empty string. Example:
/*
init_config:
instances:
  - collectors: []
    crd_collectors:
      - datadoghq.com/v1alpha1/datadogmetrics
*/
func (cb *CollectorBundle) skipImportingDefaultCollectors() bool {
	return cb.check.instance.Collectors != nil && len(cb.check.instance.Collectors) == 0
}

// addCollectorFromConfig appends a collector to the bundle based on the
// collector name specified in the check configuration.
//
// ## Normal Groups
// The following configuration keys are accepted:
//   - <collector_name> (e.g "cronjobs")
//   - <apigroup_and_version>/<collector_name> (e.g. "batch/v1/cronjobs")
//
// ## CRDs
// The following configuration keys are accepted:
//   - <apigroup_and_version>/<collector_name> (e.g. "batch/v1/cronjobs")
func (cb *CollectorBundle) addCollectorFromConfig(collectorName string, isCRD bool) {
	var (
		collector collectors.K8sCollector
		err       error
	)

	if isCRD {
		idx := strings.LastIndex(collectorName, "/")
		if idx == -1 {
			_ = cb.check.Warnf("Unsupported crd collector definition: %s. Definition needs to be of <apigroup_and_version>/<collector_name> (e.g. \"batch/v1/cronjobs\")", collectorName)
			return
		}
		groupVersion := collectorName[:idx]
		resource := collectorName[idx+1:]

		if cb.skipResources(groupVersion, resource) {
			return
		}

		if c, _ := cb.inventory.CollectorForVersion(resource, groupVersion); c != nil {
			_ = cb.check.Warnf("Ignoring CRD collector %s: use builtin collection instead", collectorName)

			return
		}
		collector, err = cb.collectorDiscovery.VerifyForCRDInventory(resource, groupVersion)
	} else if idx := strings.LastIndex(collectorName, "/"); idx != -1 {
		groupVersion := collectorName[:idx]
		name := collectorName[idx+1:]
		collector, err = cb.collectorDiscovery.VerifyForInventory(name, groupVersion, cb.inventory)
	} else {
		collector, err = cb.collectorDiscovery.VerifyForInventory(collectorName, "", cb.inventory)
	}

	if err != nil {
		_ = cb.check.Warnf("Unsupported collector: %s: %s", collectorName, err)
		return
	}

	// this is to stop multiple crds and/or people setting resources as custom resources which we already collect
	// I am using the fullName for now on purpose in case we have the same resource across 2 different groups setup
	if _, ok := cb.activatedCollectors[collector.Metadata().FullName()]; ok {
		_ = cb.check.Warnf("collector %s has already been added", collectorName) // Before using unstable info
		return
	}

	if !collector.Metadata().IsStable && !isCRD {
		_ = cb.check.Warnf("Using unstable collector: %s", collector.Metadata().FullName())
	}

	cb.activatedCollectors[collector.Metadata().FullName()] = struct{}{}
	cb.collectors = append(cb.collectors, collector)
}

// importCollectorsFromCheckConfig tries to fill the bundle with the list of
// collectors specified in the orchestrator check configuration. Returns true if
// at least one collector was set, false otherwise.
func (cb *CollectorBundle) importCollectorsFromCheckConfig() bool {
	if len(cb.check.instance.Collectors) == 0 {
		return false
	}
	for _, c := range cb.check.instance.Collectors {
		cb.addCollectorFromConfig(c, false)
	}
	return true
}

// importCRDCollectorsFromCheckConfig tries to fill the crd bundle with the list of
// collectors specified in the orchestrator crd check configuration. Returns true if
// at least one collector was set, false otherwise.
func (cb *CollectorBundle) importCRDCollectorsFromCheckConfig() bool {
	if len(cb.check.instance.CRDCollectors) == 0 {
		return false
	}

	crdCollectors := cb.check.instance.CRDCollectors
	if len(cb.check.instance.CRDCollectors) > defaultMaximumCRDs {
		crdCollectors = cb.check.instance.CRDCollectors[:defaultMaximumCRDs]
		cb.check.Warnf("Too many crd collectors are configured, will only collect the first %d collectors", defaultMaximumCRDs)
	}

	for _, c := range crdCollectors {
		cb.addCollectorFromConfig(c, true)
	}
	return true
}

// importCollectorsFromDiscovery tries to fill the bundle with the list of
// collectors discovered through resources available from the API server.
// Returns true if at least one collector was set, false otherwise.
func (cb *CollectorBundle) importCollectorsFromDiscovery() bool {
	if !cb.discoverCollectors {
		return false
	}

	collectors, err := discovery.NewAPIServerDiscoveryProvider().Discover(cb.inventory)
	if err != nil {
		_ = cb.check.Warnf("Kubernetes collector discovery failed: %s", err)
		return false
	}
	if len(collectors) == 0 {
		_ = cb.check.Warn("Kubernetes collector discovery returned no collector")
		return false
	}

	cb.collectors = append(cb.collectors, collectors...)

	return true
}

// importCollectorsFromInventory fills the bundle with the list of
// stable collectors with default versions.
func (cb *CollectorBundle) importCollectorsFromInventory() {
	cb.collectors = cb.inventory.StableCollectors()
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
	// informerSynced is a helper map which makes sure that we don't initialize the same informer twice.
	// i.e. the cluster and nodes resources share the same informer and using both can lead to a race condition activating both concurrently.
	informerSynced := map[cache.SharedInformer]struct{}{}

	for _, collector := range cb.collectors {
		collectorFullName := collector.Metadata().FullName()

		// init metrics
		skippedResources[collectorFullName] = &expvar.String{}
		skippedResourcesExpVars.Set(collectorFullName, skippedResources[collectorFullName])

		collector.Init(cb.runCfg)
		informer := collector.Informer()

		if _, found := informerSynced[informer]; !found {
			informersToSync[apiserver.InformerName(collectorFullName)] = informer
			informerSynced[informer] = struct{}{}
			// we run each enabled informer individually, because starting them through the factory
			// would prevent us from restarting them again if the check is unscheduled/rescheduled
			// see https://github.com/kubernetes/client-go/blob/3511ef41b1fbe1152ef5cab2c0b950dfd607eea7/informers/factory.go#L64-L66

			go informer.Run(cb.stopCh)
		}
	}

	errors := apiserver.SyncInformersReturnErrors(informersToSync, cb.extraSyncTimeout)

	for informerName, err := range errors {
		if err != nil {
			cb.skipCollector(informerName, err)
		}
	}

	return nil
}

func (cb *CollectorBundle) skipCollector(informerName apiserver.InformerName, err error) {
	for _, collector := range cb.collectors {
		collectorFullName := collector.Metadata().FullName()
		if apiserver.InformerName(collectorFullName) == informerName {
			collector.Metadata().IsSkipped = true
			collector.Metadata().SkippedReason = err.Error()

			// emit metrics
			skippedResources[collectorFullName].Set(err.Error())
			tlmSkippedResources.Inc(collectorFullName)
		}
	}
}

// Run is used to sequentially run all collectors in the bundle.
func (cb *CollectorBundle) Run(sender sender.Sender) {

	// Start a thread to buffer manifests and kill it when the check is finished.
	if cb.runCfg.Config.IsManifestCollectionEnabled && cb.manifestBuffer.Cfg.BufferedManifestEnabled {
		cb.manifestBuffer.Start(sender)
		defer cb.manifestBuffer.Stop()
	}

	for _, collector := range cb.collectors {
		if collector.Metadata().IsSkipped {
			_ = cb.check.Warnf("Collector %s is skipped: %s", collector.Metadata().FullName(), collector.Metadata().SkippedReason)
			continue
		}

		runStartTime := time.Now()

		result, err := collector.Run(cb.runCfg)
		if err != nil {
			_ = cb.check.Warnf("Collector %s failed to run: %s", collector.Metadata().FullName(), err.Error())
			continue
		}

		runDuration := time.Since(runStartTime)
		log.Debugf("Collector %s run stats: listed=%d processed=%d messages=%d duration=%s", collector.Metadata().FullName(), result.ResourcesListed, result.ResourcesProcessed, len(result.Result.MetadataMessages), runDuration)

		nt := collector.Metadata().NodeType
		orchestrator.SetCacheStats(result.ResourcesListed, len(result.Result.MetadataMessages), nt)

		if collector.Metadata().IsMetadataProducer { // for CR and CRD we don't have metadata but only manifests
			sender.OrchestratorMetadata(result.Result.MetadataMessages, cb.check.clusterID, int(nt))
		}

		if cb.runCfg.Config.IsManifestCollectionEnabled {
			if cb.manifestBuffer.Cfg.BufferedManifestEnabled && collector.Metadata().SupportsManifestBuffering {
				BufferManifestProcessResult(result.Result.ManifestMessages, cb.manifestBuffer)
			} else {
				sender.OrchestratorManifest(result.Result.ManifestMessages, cb.check.clusterID)
			}
		}
	}
}

func (cb *CollectorBundle) skipResources(groupVersion, resource string) bool {
	if groupVersion == "v1" && (resource == "secrets" || resource == "configmaps") {
		cb.check.Warnf("Skipping collector: %s/%s, we don't support collecting it for now as it can contain sensitive data", groupVersion, resource)
		return true
	}
	return false
}

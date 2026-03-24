// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

//nolint:revive // TODO(CAPP) Fix revive linter
package orchestrator

import (
	"context"
	"expvar"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/tools/cache"
)

const (
	defaultExtraSyncTimeout = 60 * time.Second
	defaultMaximumCRDs      = 100
	datadogAPIGroup         = "datadoghq.com"
	ArgoAPIGroup            = "argoproj.io"
	FluxAPIGroup            = "source.toolkit.fluxcd.io"
	FluxKustomizeAPIGroup   = "kustomize.toolkit.fluxcd.io"
	KarpenterAPIGroup       = "karpenter.sh"
	KarpenterAWSAPIGroup    = "karpenter.k8s.aws"
	KarpenterAzureAPIGroup  = "karpenter.azure.com"
	EKSAPIGroup             = "eks.amazonaws.com"
)

var (
	skippedResourcesExpVars = expvar.NewMap("orchestrator-skipped-resources")
	skippedResources        = map[string]*expvar.String{}

	tlmSkippedResources = telemetry.NewCounter("orchestrator", "skipped_resources", []string{"name"}, "Skipped resources in orchestrator check")
)

// CollectorBundle is a container for a group of collectors. It provides a way
// to easily run them all.
type CollectorBundle struct {
	check                    *OrchestratorCheck
	collectors               []collectors.K8sCollector
	discoverCollectors       bool
	extraSyncTimeout         time.Duration
	inventory                *inventory.CollectorInventory
	stopCh                   chan struct{}
	runCfg                   *collectors.CollectorRunConfig
	manifestBuffer           *ManifestBuffer
	collectorDiscovery       *discovery.DiscoveryCollector
	activatedCollectors      map[string]struct{}
	terminatedResourceBundle *TerminatedResourceBundle
	initializeOnce           sync.Once
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
	runCfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient:                   chk.apiClient,
			OrchestratorInformerFactory: chk.orchestratorInformerFactory,
		},
		ClusterID:    chk.clusterID,
		Config:       chk.orchestratorConfig,
		MsgGroupRef:  chk.groupID,
		AgentVersion: chk.agentVersion,
		StopCh:       chk.stopCh,
	}
	terminatedResourceRunCfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: runCfg.K8sCollectorRunConfig,
		ClusterID:             runCfg.ClusterID,
		Config:                runCfg.Config,
		MsgGroupRef:           runCfg.MsgGroupRef,
		TerminatedResources:   true,
	}

	manifestBuffer := NewManifestBuffer(chk)
	terminatedResourceBundle := NewTerminatedResourceBundle(chk, terminatedResourceRunCfg, manifestBuffer)
	runCfg.TerminatedResourceHandler = terminatedResourceBundle.Add

	bundle := &CollectorBundle{
		discoverCollectors:       chk.orchestratorConfig.CollectorDiscoveryEnabled,
		check:                    chk,
		inventory:                inventory.NewCollectorInventory(chk.cfg, chk.wlmStore, chk.tagger),
		runCfg:                   runCfg,
		stopCh:                   chk.stopCh,
		manifestBuffer:           manifestBuffer,
		collectorDiscovery:       discovery.NewDiscoveryCollectorForInventory(),
		activatedCollectors:      map[string]struct{}{},
		terminatedResourceBundle: terminatedResourceBundle,
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

	defer cb.importBuiltinCollectors()

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
func (cb *CollectorBundle) Initialize() {
	cb.initializeOnce.Do(cb.initialize)
}

func (cb *CollectorBundle) initialize() {
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

		// special case of improved terminated pod collector that is not using an informer
		// TODO: improve the initialization logic to avoid leaking collector implementation details to the bundle.
		if informer == nil {
			continue
		}

		if _, found := informerSynced[informer]; !found {
			informersToSync[apiserver.InformerName(collectorFullName)] = informer
			informerSynced[informer] = struct{}{}

			// add event handlers for terminated resources
			if collector.Metadata().SupportsTerminatedResourceCollection {
				if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
					DeleteFunc: cb.terminatedResourceHandler(collector),
				}); err != nil {
					log.Warnf("Failed to add delete event handler for %s: %s", collectorFullName, err)
				}
			}

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

	cb.terminatedResourceBundle.Run()
}

func (cb *CollectorBundle) skipResources(groupVersion, resource string) bool {
	if groupVersion == "v1" && (resource == "secrets" || resource == "configmaps") {
		cb.check.Warnf("Skipping collector: %s/%s, we don't support collecting it for now as it can contain sensitive data", groupVersion, resource)
		return true
	}
	return false
}

func (cb *CollectorBundle) terminatedResourceHandler(collector collectors.K8sCollector) func(obj interface{}) {
	return func(obj interface{}) {
		cb.terminatedResourceBundle.Add(collector, obj)
	}
}

// GetTerminatedResourceBundle returns the terminated resource bundle.
func (cb *CollectorBundle) GetTerminatedResourceBundle() *TerminatedResourceBundle {
	return cb.terminatedResourceBundle
}

// EnableTerminatedResourceBundle enables the terminated resource bundle if the feature is enabled.
func (cb *CollectorBundle) EnableTerminatedResourceBundle() {
	if pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.terminated_resources.enabled") {
		cb.terminatedResourceBundle.Enable()
	}
}

// importBuiltinCollectors imports the builtin collectors into the bundle.
func (cb *CollectorBundle) importBuiltinCollectors() {
	// add builtin CR collectors
	builtinCollectors := cb.getBuiltinCustomResourceCollectors()

	// add terminated pod collector
	terminatedPodCollector := cb.getTerminatedPodCollector()
	if terminatedPodCollector != nil {
		builtinCollectors = append(builtinCollectors, terminatedPodCollector)
	}

	// add builtin collectors and check if they are already activated
	for _, collector := range builtinCollectors {
		if _, ok := cb.activatedCollectors[collector.Metadata().FullName()]; ok {
			log.Debugf("collector %s has already been added", collector.Metadata().FullName())
			continue
		}

		cb.activatedCollectors[collector.Metadata().FullName()] = struct{}{}
		log.Debugf("import builtin collector: %s", collector.Metadata().FullName())
		cb.collectors = append(cb.collectors, collector)
	}
}

// builtinCRDConfig represents the configuration for a built-in custom resource definition.
type builtinCRDConfig struct {
	// group is the API group name for the custom resource
	group string
	// kind is the resource kind name
	kind string
	// enabled indicates whether collection of this CRD is enabled
	enabled bool
	// preferredVersion is the preferred API version we want to collect for this custom resource
	preferredVersion string
	// fallbackVersions is a list of versions that we can fall back to in order when preferredVersion is unavailable
	fallbackVersions []string
}

// newBuiltinCRDConfig creates a new builtinCRDConfig.
func newBuiltinCRDConfig(group, kind string, enabled bool, preferredVersion string, fallbackVersions ...string) builtinCRDConfig {
	return builtinCRDConfig{
		group:            group,
		preferredVersion: preferredVersion,
		fallbackVersions: fallbackVersions,
		kind:             kind,
		enabled:          enabled,
	}
}

// newBuiltinCRDConfigs returns the configuration for all built-in CRDs.
func newBuiltinCRDConfigs() []builtinCRDConfig {
	isOOTBCRDEnabled := pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.custom_resources.ootb.enabled")

	return []builtinCRDConfig{
		// Datadog resources
		newBuiltinCRDConfig(datadogAPIGroup, "datadogslos", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(datadogAPIGroup, "datadogdashboards", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(datadogAPIGroup, "datadogagentprofiles", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(datadogAPIGroup, "datadogmonitors", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(datadogAPIGroup, "datadogmetrics", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(datadogAPIGroup, "datadogpodautoscalers", isOOTBCRDEnabled, "v1alpha2"),
		newBuiltinCRDConfig(datadogAPIGroup, "datadogagents", isOOTBCRDEnabled, "v2alpha1"),

		// Argo resources
		newBuiltinCRDConfig(ArgoAPIGroup, "rollouts", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(ArgoAPIGroup, "applications", isOOTBCRDEnabled, "v1alpha1"),
		newBuiltinCRDConfig(ArgoAPIGroup, "applicationsets", isOOTBCRDEnabled, "v1alpha1"),
		// appprojects also exists, but unclear if they are need for resource location identification.

		// Flux resources
		newBuiltinCRDConfig(FluxAPIGroup, "buckets", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(FluxAPIGroup, "helmcharts", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(FluxAPIGroup, "externalartifacts", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(FluxAPIGroup, "gitrepositories", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(FluxAPIGroup, "helmrepositories", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(FluxAPIGroup, "ocirepositories", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(FluxKustomizeAPIGroup, "kustomizations", isOOTBCRDEnabled, "v1"),

		// Karpenter resources (empty kind = all resources in group)
		newBuiltinCRDConfig(KarpenterAPIGroup, "", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(KarpenterAWSAPIGroup, "", isOOTBCRDEnabled, "v1"),
		newBuiltinCRDConfig(KarpenterAzureAPIGroup, "", isOOTBCRDEnabled, "v1beta1"),

		// EKS Auto Mode resources (for now only nodeclasses, but we can easily add more in the future if needed)
		newBuiltinCRDConfig(EKSAPIGroup, "nodeclasses", isOOTBCRDEnabled, "v1", "v1beta1"),
	}
}

// getBuiltinCustomResourceCollectors returns the list of builtin custom resource collectors.
func (cb *CollectorBundle) getBuiltinCustomResourceCollectors() []collectors.K8sCollector {
	// Check if the CRD collector is present, if not, return an empty list
	// This is to ensure that we only collect CRs if the CRD collector is present
	if !cb.hasCRDCollector() {
		return []collectors.K8sCollector{}
	}

	crCollectors := make([]collectors.K8sCollector, 0, 10)
	for _, builtinCustomResource := range newBuiltinCRDConfigs() {
		crCollectors = append(crCollectors, cb.collectorsForBuiltinCRD(builtinCustomResource)...)
	}

	crCollectors = filterCRCollectorsByPermission(crCollectors, cb.isForbidden)

	return crCollectors
}

// collectorsForBuiltinCRD returns the list of collectors for a built-in CRD.
func (cb *CollectorBundle) collectorsForBuiltinCRD(builtinCustomResource builtinCRDConfig) []collectors.K8sCollector {
	if !builtinCustomResource.enabled {
		return nil
	}

	version, ok := cb.collectorDiscovery.OptimalVersion(builtinCustomResource.group, builtinCustomResource.preferredVersion, builtinCustomResource.fallbackVersions)
	if !ok {
		log.Infof("Skipping built-in CR collector: no supported version found for %s/%s (preferred: %s, fallback: %s)",
			builtinCustomResource.group, builtinCustomResource.kind, builtinCustomResource.preferredVersion, builtinCustomResource.fallbackVersions)
		return nil
	}

	crCollectors := make([]collectors.K8sCollector, 0, 10)
	crs := cb.collectorDiscovery.List(builtinCustomResource.group, version, builtinCustomResource.kind)
	for _, c := range crs {
		collector, err := cb.collectorDiscovery.VerifyForCRDInventory(c.Kind, c.GroupVersion)
		if err != nil {
			log.Infof("Unsupported built-in CR collector: %s/%s: %s", c.GroupVersion, c.Kind, err)
			continue
		}

		crCollectors = append(crCollectors, collector)
	}
	return crCollectors
}

// hasCRDCollector returns true if the CRD collector is present.
func (cb *CollectorBundle) hasCRDCollector() bool {
	for _, collector := range cb.collectors {
		if collector.Metadata().Name == utilTypes.CrdName {
			return true
		}
	}
	return false
}

// getTerminatedPodCollector returns the terminated pod collector if the unassigned pod collector is present and the terminated pod collector is stable.
func (cb *CollectorBundle) getTerminatedPodCollector() collectors.K8sCollector {
	hasUnassignedPodCollector := false
	hasTerminatedPodCollector := false
	for _, collector := range cb.collectors {
		if collector.Metadata().Name == utilTypes.PodName {
			hasUnassignedPodCollector = true
		}
		if collector.Metadata().Name == utilTypes.TerminatedPodName {
			hasTerminatedPodCollector = true
		}
	}

	// add terminated pod collector if unassigned pod collector is added and terminated pod collector is stable
	if hasUnassignedPodCollector && !hasTerminatedPodCollector {
		terminatedPodCollector, err := cb.collectorDiscovery.VerifyForInventory(utilTypes.TerminatedPodName, "", cb.inventory)
		if err != nil {
			log.Warnf("Unabled to add terminated pod collector: %s", err)
			return nil
		}
		if terminatedPodCollector.Metadata().IsStable {
			return terminatedPodCollector
		}
	}
	return nil
}

// isForbidden runs a single List request to check if cluster agent is forbidden to list the given resource
func (cb *CollectorBundle) isForbidden(gvr schema.GroupVersionResource) bool {
	_, err := cb.runCfg.APIClient.DynamicCl.Resource(gvr).List(context.Background(), metav1.ListOptions{})
	return errors.IsForbidden(err)
}

// filterCRCollectorsByPermission filters collectors based on permissions, keeping only those with sufficient access.
func filterCRCollectorsByPermission(crCollectors []collectors.K8sCollector, isForbidden func(gvr schema.GroupVersionResource) bool) []collectors.K8sCollector {
	filteredCollectors := make([]collectors.K8sCollector, 0, len(crCollectors))
	for _, c := range crCollectors {
		if cr, ok := c.(*k8s.CRCollector); ok {
			if isForbidden(cr.GetGRV()) {
				log.Infof("Skipping built-in collector due to insufficient permissions: %s", cr.GetGRV().String())
				continue
			}
			filteredCollectors = append(filteredCollectors, c)
		}
	}
	return filteredCollectors
}

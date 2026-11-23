// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

//go:build kubeapiserver

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.yaml.in/yaml/v3"
	"k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	vpai "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggerutils "github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
	"github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// CheckName is the name of the check
	CheckName = orchestrator.CheckName

	maximumWaitForAPIServer = 10 * time.Second
	collectionInterval      = 10 * time.Second
	defaultResyncInterval   = 300 * time.Second
)

// OrchestratorInstance is the config of the orchestrator check instance.
type OrchestratorInstance struct {
	LeaderSkip bool `yaml:"skip_leader_election"`
	// Collectors defines the resource type collectors.
	// Example: Enable services and nodes collectors.
	// collectors:
	//   - nodes
	//   - services
	Collectors []string `yaml:"collectors"`
	// CRDCollectors defines collectors for custom Kubernetes resource types.
	// crd_collectors:
	//   - datadoghq.com/v1alpha1/datadogmetrics
	//   - stable.example.com/v1/crontabs
	CRDCollectors           []string `yaml:"crd_collectors"`
	ExtraSyncTimeoutSeconds int      `yaml:"extra_sync_timeout_seconds"`
}

func (c *OrchestratorInstance) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// OrchestratorCheck wraps the config and the informers needed to run the check
type OrchestratorCheck struct {
	core.CheckBase
	orchestratorConfig          *orchcfg.OrchestratorConfig
	instance                    *OrchestratorInstance
	collectorBundle             *CollectorBundle
	wlmStore                    workloadmeta.Component
	cfg                         configcomp.Component
	tagger                      tagger.Component
	stopCh                      chan struct{}
	clusterID                   string
	groupID                     *atomic.Int32
	isCLCRunner                 bool
	apiClient                   *apiserver.APIClient
	orchestratorInformerFactory *collectors.OrchestratorInformerFactory
	agentVersion                *model.AgentVersion

	// manifestBundle sends the manifests when the dedicated manifest pipeline is enabled. collectorBundle then sends
	// structured metadata only.
	manifestBundle *CollectorBundle
	// runningInformers makes sure that an informer shared by both bundles runs once. Like
	// CollectorBundle.startedInformers, it is keyed by the informer store.
	runningInformers map[cache.Store]struct{}
	// discovery is the API discovery of both bundles.
	discovery *sharedDiscovery
	// configMu protects the payload tags of the configuration, which Cancel reads from another goroutine.
	configMu sync.Mutex
	// deps are the services that the check reaches outside of the process.
	deps checkDependencies
}

// checkDependencies are the services that the check reaches outside of the process. Tests replace them to run the
// check against fake clients.
type checkDependencies struct {
	waitForAPIClient         func(ctx context.Context) (*apiserver.APIClient, error)
	runLeaderElection        func() (leader string, err error)
	serverGroupsAndResources discovery.ServerGroupsAndResourcesFunc
}

// withDefaults returns the dependencies, with the production services for the ones that are not set.
func (d checkDependencies) withDefaults() checkDependencies {
	if d.waitForAPIClient == nil {
		d.waitForAPIClient = apiserver.WaitForAPIClient
	}
	if d.runLeaderElection == nil {
		d.runLeaderElection = cluster.RunLeaderElection
	}
	if d.serverGroupsAndResources == nil {
		d.serverGroupsAndResources = discovery.GetServerGroupsAndResources
	}
	return d
}

func newOrchestratorCheck(base core.CheckBase, instance *OrchestratorInstance, cfg configcomp.Component, wlmStore workloadmeta.Component, tagger tagger.Component) *OrchestratorCheck {
	agentVersion, err := version.Agent()
	if err != nil {
		log.Warnf("Failed to get agent version: %s", err)
	}

	return &OrchestratorCheck{
		CheckBase:   base,
		instance:    instance,
		wlmStore:    wlmStore,
		tagger:      tagger,
		cfg:         cfg,
		stopCh:      make(chan struct{}),
		groupID:     atomic.NewInt32(rand.Int31()),
		isCLCRunner: helper.IsCLCRunner(pkgconfigsetup.Datadog()),
		agentVersion: &model.AgentVersion{
			Major:  agentVersion.Major,
			Minor:  agentVersion.Minor,
			Patch:  agentVersion.Patch,
			Pre:    agentVersion.Pre,
			Meta:   agentVersion.Meta,
			Commit: agentVersion.Commit,
		},
		runningInformers: map[cache.Store]struct{}{},
	}
}

// Factory creates a new check factory
func Factory(wlm workloadmeta.Component, cfg configcomp.Component, tagger tagger.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check { return newCheck(cfg, wlm, tagger) })
}

func newCheck(cfg configcomp.Component, wlm workloadmeta.Component, tagger tagger.Component) check.Check {
	return newOrchestratorCheck(
		core.NewCheckBase(CheckName),
		&OrchestratorInstance{},
		cfg,
		wlm,
		tagger,
	)
}

// Interval returns the scheduling time for the check.
func (o *OrchestratorCheck) Interval() time.Duration {
	return collectionInterval
}

// Configure configures the orchestrator check
func (o *OrchestratorCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string, provider string) error {
	o.BuildID(integrationConfigDigest, config, initConfig)

	err := o.CommonConfigure(senderManager, initConfig, config, source, provider)
	if err != nil {
		return err
	}

	// Retrieves tags on the check config (applicable when scheduled on CLC)
	checkConfigExtraTags, err := getTags(initConfig, config)
	if err != nil {
		return fmt.Errorf("could not parse tags from check config: %w", err)
	}

	// Retrieves tags from the tagger (applicable when scheduled on DCA)
	taggerExtraTags, err := o.tagger.GlobalTags(types.LowCardinality)
	if err != nil {
		return fmt.Errorf("could not get global tags from tagger: %w", err)
	}

	extraTags := make([]string, 0, len(checkConfigExtraTags)+len(taggerExtraTags))
	extraTags = append(extraTags, checkConfigExtraTags...)
	extraTags = append(extraTags, taggerExtraTags...)

	// On a Cluster Check Runner the dispatched check configuration can already
	// carry the mark, so append it only when it is absent.
	extraTags = taggerutils.AppendUniqueTags(extraTags, o.tagger.GetInfraTags()...)

	o.orchestratorConfig = orchcfg.NewDefaultOrchestratorConfig(extraTags)

	err = o.orchestratorConfig.Load()
	if err != nil {
		return err
	}

	if !o.orchestratorConfig.OrchestrationCollectionEnabled {
		return errors.New("orchestrator check is configured but the feature is disabled")
	}
	if o.orchestratorConfig.KubeClusterName == "" {
		return errors.New("orchestrator check is configured but the cluster name is empty")
	}

	// load instance level config
	err = o.instance.parse(config)
	if err != nil {
		_ = log.Errorc("could not parse check instance config", orchestrator.ExtraLogContext...)
		return err
	}

	o.clusterID, err = clustername.GetClusterID()
	if err != nil {
		return err
	}

	// Reuse the common API Server client to share the cache
	// Due to how init is done, we cannot use GetAPIClient in `Run()` method
	// So we are waiting for a reasonable amount of time here in case.
	// We cannot wait forever as there's no way to be notified of shutdown
	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()

	o.apiClient, err = o.deps.withDefaults().waitForAPIClient(apiCtx)
	if err != nil {
		return err
	}

	o.orchestratorInformerFactory = getOrchestratorInformerFactory(o.apiClient)

	o.discovery = newSharedDiscovery(o.deps.withDefaults().serverGroupsAndResources)

	// Create the bundles for the check.
	if o.orchestratorConfig.ManifestPipelineEnabled {
		log.Infoc("Sending manifests from the dedicated manifest pipeline", orchestrator.ExtraLogContext...)
		o.collectorBundle = NewCollectorBundle(o, collectors.MetadataRole)
		o.manifestBundle = NewCollectorBundle(o, collectors.ManifestRole)
	} else {
		o.collectorBundle = NewCollectorBundle(o, collectors.MetadataAndManifestRole)
	}

	return nil
}

// bundles returns the collector bundles of the check.
func (o *OrchestratorCheck) bundles() []*CollectorBundle {
	if o.collectorBundle == nil {
		return nil
	}
	if o.manifestBundle == nil {
		return []*CollectorBundle{o.collectorBundle}
	}
	return []*CollectorBundle{o.collectorBundle, o.manifestBundle}
}

// runInformer runs an informer until stopCh is closed, unless another bundle already runs it.
func (o *OrchestratorCheck) runInformer(informer cache.SharedInformer, stopCh chan struct{}) {
	store := informer.GetStore()
	if _, found := o.runningInformers[store]; found {
		return
	}
	o.runningInformers[store] = struct{}{}
	go informer.Run(stopCh)
}

// Run runs the orchestrator check
func (o *OrchestratorCheck) Run() error {
	o.refreshInfraTags()
	if o.discovery != nil {
		o.discovery.nextRun()
	}

	if ok, err := o.IsLeader(); !ok {
		// Stop queueing terminated resources if not leader, so that informer delete handlers queue nothing
		for _, bundle := range o.bundles() {
			bundle.DisableTerminatedResources()
		}
		return err
	}

	// Start queueing terminated resources in every bundle before any bundle work, so that the bundles queue the same
	// deletions.
	for _, bundle := range o.bundles() {
		bundle.EnableTerminatedResources()
	}

	for _, bundle := range o.bundles() {
		bundle.RefreshDiscovery()

		// Initialize collectors
		bundle.Initialize()
	}

	// access serializer
	sender, err := o.GetSender()
	if err != nil {
		return err
	}

	// Run all collectors.
	for _, bundle := range o.bundles() {
		bundle.Run(sender)
	}

	return nil
}

// refreshInfraTags adds the infrastructure mode tags to the payload tags when
// they were not available at Configure time. On a Cluster Check Runner they come
// from the Cluster Agent's tagger stream, which can connect after the check is
// configured.
//
// Other goroutines read the tags in two places: the manifest buffer, which only
// runs inside a collector bundle run and so never overlaps this call, and the
// terminated resource flush that Cancel triggers, which configMu covers.
func (o *OrchestratorCheck) refreshInfraTags() {
	infraTags := o.tagger.GetInfraTags()
	missing := slices.ContainsFunc(infraTags, func(tag string) bool {
		return !slices.Contains(o.orchestratorConfig.ExtraTags, tag)
	})
	if !missing {
		return
	}

	o.configMu.Lock()
	o.orchestratorConfig.ExtraTags = taggerutils.AppendUniqueTags(o.orchestratorConfig.ExtraTags, infraTags...)
	o.configMu.Unlock()
	log.Infoc(fmt.Sprintf("Adding infrastructure mode tags %v to orchestrator payloads", infraTags), orchestrator.ExtraLogContext...)
}

// Cancel cancels the orchestrator check
func (o *OrchestratorCheck) Cancel() {
	log.Infoc(fmt.Sprintf("Shutting down informers used by the check '%s'", o.ID()), orchestrator.ExtraLogContext...)
	close(o.stopCh)
	// send the queued terminated resources and stop queueing
	o.configMu.Lock()
	defer o.configMu.Unlock()
	for _, bundle := range o.bundles() {
		bundle.DisableTerminatedResources()
	}
}

func getOrchestratorInformerFactory(apiClient *apiserver.APIClient) *collectors.OrchestratorInformerFactory {
	unassignedPodsTweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", "").String()
	}
	helmReleasesTweakListOptions := func(options *metav1.ListOptions) {
		options.LabelSelector = registry.HelmReleaseLabelSelector
	}

	of := &collectors.OrchestratorInformerFactory{
		InformerFactory:              informers.NewSharedInformerFactoryWithOptions(apiClient.InformerCl, defaultResyncInterval),
		CRDInformerFactory:           externalversions.NewSharedInformerFactory(apiClient.CRDInformerClient, defaultResyncInterval),
		DynamicInformerFactory:       dynamicinformer.NewDynamicSharedInformerFactory(apiClient.DynamicInformerCl, defaultResyncInterval),
		VPAInformerFactory:           vpai.NewSharedInformerFactory(apiClient.VPAInformerClient, defaultResyncInterval),
		UnassignedPodInformerFactory: informers.NewSharedInformerFactoryWithOptions(apiClient.InformerCl, defaultResyncInterval, informers.WithTweakListOptions(unassignedPodsTweakListOptions)),
	}
	of.HelmReleaseConfigMapInformerFactory = informers.NewSharedInformerFactoryWithOptions(apiClient.InformerCl, defaultResyncInterval, informers.WithTweakListOptions(helmReleasesTweakListOptions))
	of.SecretMetadataInformerFactory = metadatainformer.NewSharedInformerFactory(apiClient.MetadataInformerCl, defaultResyncInterval)

	// When enabled, Helm release ConfigMaps are reported by the Helm release collector, with their data.
	var configMapTweakListOptions metadatainformer.TweakListOptionsFunc
	if pkgconfigsetup.Datadog().GetBool("orchestrator_explorer.helm_releases.configmaps.enabled") {
		configMapTweakListOptions = func(options *metav1.ListOptions) {
			options.LabelSelector = registry.NonHelmReleaseLabelSelector
		}
	}
	of.ConfigMapMetadataInformerFactory = metadatainformer.NewFilteredSharedInformerFactory(apiClient.MetadataInformerCl, defaultResyncInterval, metav1.NamespaceAll, configMapTweakListOptions)

	return of
}

// getTags extracts tags from the configurations
func getTags(initConfig, config integration.Data) ([]string, error) {
	initCommonOptions := integration.CommonInstanceConfig{}
	err := yaml.Unmarshal(initConfig, &initCommonOptions)
	if err != nil {
		return nil, err
	}

	commonOptions := integration.CommonInstanceConfig{}
	err = yaml.Unmarshal(config, &commonOptions)
	if err != nil {
		return nil, err
	}

	var tags []string
	if commonOptions.Tags != nil {
		tags = append(tags, commonOptions.Tags...)
	}
	if initCommonOptions.Tags != nil {
		tags = append(tags, initCommonOptions.Tags...)
	}

	return tags, nil
}

// IsLeader checks if the orchestrator check is running on the leader node
func (o *OrchestratorCheck) IsLeader() (bool, error) {
	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// we also do a safety check for dedicated runners to avoid trying the leader election
	if o.isCLCRunner && o.instance.LeaderSkip {
		return true, nil
	}

	// Only run if Leader Election is enabled.
	if !pkgconfigsetup.Datadog().GetBool("leader_election") {
		return false, log.Errorc("Leader Election not enabled. The cluster-agent will not run the check.", orchestrator.ExtraLogContext...)
	}

	leader, err := o.deps.withDefaults().runLeaderElection()
	if err != nil {
		if err == apiserver.ErrNotLeader {
			log.Debugf("Not leader (leader is %q). Skipping the Orchestrator check", leader)
			return false, nil
		}
		log.Warn("Leader Election error. Not running the Orchestrator check.")
		return false, err
	}

	log.Tracef("Current leader: %q, running the Orchestrator check", leader)
	return true, nil
}

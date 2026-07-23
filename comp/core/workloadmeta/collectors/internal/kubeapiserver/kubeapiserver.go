// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeapiserver implements the kubeapiserver Workloadmeta collector.
package kubeapiserver

import (
	"context"
	"strings"
	"time"

	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "kubeapiserver"
	componentName = "workloadmeta-kubeapiserver"
	noResync      = time.Duration(0)
)

type dependencies struct {
	fx.In

	Config          config.Component
	AutoscalingGate *autoscalinggate.Gate
}

// storeGenerator returns a new store specific to a given resource
type storeGenerator func(context.Context, workloadmeta.Component, config.Reader, kubernetes.Interface) (*cache.Reflector, *reflectorStore)

func shouldHavePodStore(cfg config.Reader) bool {
	return podsRequiredAtStartup(cfg) || cfg.GetBool("autoscaling.workload.enabled")
}

// podsRequiredAtStartup returns true when some feature needs the pod store to
// be available from startup. Workload autoscaling does not. It can defer the
// start via the autoscaling gate.
func podsRequiredAtStartup(cfg config.Reader) bool {
	metadataAsTags := configutils.GetMetadataAsTags(cfg)
	return cfg.GetBool("cluster_agent.collect_kubernetes_tags") ||
		cfg.GetBool("autoscaling.cluster.spot.enabled") ||
		len(metadataAsTags.GetPodLabelsAsTags()) > 0 ||
		len(metadataAsTags.GetPodAnnotationsAsTags()) > 0
}

func shouldHaveDeploymentStore(cfg config.Reader) bool {
	metadataAsTags := configutils.GetMetadataAsTags(cfg)
	hasDeploymentsLabelsAsTags := len(metadataAsTags.GetResourcesLabelsAsTags()["deployments.apps"]) > 0
	hasDeploymentsAnnotationsAsTags := len(metadataAsTags.GetResourcesAnnotationsAsTags()["deployments.apps"]) > 0

	return cfg.GetBool("language_detection.enabled") && cfg.GetBool("language_detection.reporting.enabled") || hasDeploymentsLabelsAsTags || hasDeploymentsAnnotationsAsTags
}

func metadataCollectionGVRs(cfg config.Reader, discoveryClient discovery.DiscoveryInterface) ([]schema.GroupVersionResource, error) {
	return getGVRsForRequestedResources(discoveryClient, resourcesWithMetadataCollectionEnabled(cfg))
}

func resourcesWithMetadataCollectionEnabled(cfg config.Reader) []string {
	resources := append(
		resourcesWithRequiredMetadataCollection(cfg),
		resourcesWithExplicitMetadataCollectionEnabled(cfg)...,
	)

	// Remove duplicates and return
	return cleanDuplicateVersions(resources)
}

// resourcesWithRequiredMetadataCollection returns the list of resources that we
// need to collect metadata from in order to make other enabled features work
func resourcesWithRequiredMetadataCollection(cfg config.Reader) []string {
	// resources that we need to collect metadata from in order to make other enabled features work
	var res []string

	metadataAsTags := configutils.GetMetadataAsTags(cfg)

	for groupResource, labelsAsTags := range metadataAsTags.GetResourcesLabelsAsTags() {

		if strings.HasPrefix(groupResource, "pods") || strings.HasPrefix(groupResource, "deployments") || len(labelsAsTags) == 0 {
			continue
		}
		requestedResource := groupResourceToGVRString(groupResource)
		if requestedResource != "" {
			res = append(res, requestedResource)
		}
	}

	for groupResource, annotationsAsTags := range metadataAsTags.GetResourcesAnnotationsAsTags() {
		if strings.HasPrefix(groupResource, "pods") || strings.HasPrefix(groupResource, "deployments") || len(annotationsAsTags) == 0 {
			continue
		}
		requestedResource := groupResourceToGVRString(groupResource)
		if requestedResource != "" {
			res = append(res, requestedResource)
		}
	}

	for _, groupResource := range resourcesForAPMConfig(cfg) {
		requestedResource := groupResourceToGVRString(groupResource)
		if requestedResource != "" {
			res = append(res, requestedResource)
		}
	}

	for _, groupResource := range resourcesForCSIDetection(cfg) {
		requestedResource := groupResourceToGVRString(groupResource)
		if requestedResource != "" {
			res = append(res, requestedResource)
		}
	}

	return res
}

// resourcesWithExplicitMetadataCollectionEnabled returns the list of resources
// to collect metadata from according to the config options that configure
// metadata collection
// Pods, Deployments and Nodes are excluded if they have their separate stores and informers
// in order to avoid having two collectors collecting the same data.
func resourcesWithExplicitMetadataCollectionEnabled(cfg config.Reader) []string {
	if !cfg.GetBool("cluster_agent.kube_metadata_collection.enabled") {
		return nil
	}

	var resources []string
	requestedResources := cfg.GetStringSlice("cluster_agent.kube_metadata_collection.resources")
	for _, resource := range requestedResources {
		if strings.HasSuffix(resource, "pods") {
			log.Debugf("skipping pods from metadata collection because a separate pod store is initialised in workload metadata store.")
			continue
		}

		if strings.HasSuffix(resource, "deployments") {
			log.Debugf("skipping deployments from metadata collection because a separate deployment store is initialised in workload metadata store.")
			continue
		}

		if strings.HasSuffix(resource, "nodes") {
			log.Debugf("skipping nodes from metadata collection because a separate node store is initialised in workload metadata store.")
			continue
		}

		resources = append(resources, resource)
	}

	return resources
}

// resourcesForAPMConfig returns the list of resources to collect metadata from
// for the auto instrumentation configuration. Namespaces are collected in order
// to utilize namespace labels for target based configuration and to determine
// pod security policies to apply to restricted namespaces.
func resourcesForAPMConfig(cfg config.Reader) []string {
	// If APM is not enabled, we don't need to collect any resources for the
	// auto instrumentation configuration.
	apmEnabled := cfg.GetBool("apm_config.instrumentation.enabled")
	if !apmEnabled {
		return nil
	}

	return []string{"namespaces"}
}

// resourcesForCSIDetection returns the list of resources to collect metadata
// from for the APM auto-instrumentation library injection AutoProvider.
//
// When CSI auto-detection is enabled, the AutoProvider needs to know whether
// the Datadog CSI driver is registered in the cluster and has APM SSI
// capabilities advertised on its annotations, in order to choose between
// the CSI- and init-container-based library injection providers.
func resourcesForCSIDetection(cfg config.Reader) []string {
	if !cfg.GetBool("admission_controller.enabled") ||
		!cfg.GetBool("admission_controller.auto_instrumentation.enabled") ||
		!cfg.GetBool("apm_config.instrumentation.csi_driver_detection_enabled") {
		return nil
	}

	return []string{"csidrivers.storage.k8s.io"}
}

type collector struct {
	id              string
	catalog         workloadmeta.AgentType
	config          config.Reader
	autoscalingGate *autoscalinggate.Gate
}

// NewCollector returns a kubeapiserver CollectorProvider that instantiates its colletor
func NewCollector(deps dependencies) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:              collectorID,
			catalog:         workloadmeta.ClusterAgent,
			config:          deps.Config,
			autoscalingGate: deps.AutoscalingGate,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, wlmetaStore workloadmeta.Component) error {
	var objectStores []cache.ResourceEventHandlerRegistration

	apiserverClient, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}
	client := apiserverClient.InformerCl

	metadataclient, err := apiserverClient.MetadataClient()
	if err != nil {
		return err
	}

	// Initialize metadata collection informers
	gvrs, err := metadataCollectionGVRs(c.config, client.Discovery())

	if err != nil {
		log.Errorf("failed to discover Group and Version of requested resources: %v", err)
	} else {
		for _, gvr := range gvrs {
			reflector, store := newMetadataStore(ctx, wlmetaStore, c.config, metadataclient, gvr)
			objectStores = append(objectStores, store)
			go reflector.Run(ctx.Done())
		}
	}

	nodeReflector, nodeStore := newNodeStore(ctx, wlmetaStore, c.config, client)
	objectStores = append(objectStores, nodeStore)
	go nodeReflector.Run(ctx.Done())

	if shouldHavePodStore(c.config) {
		autoscalingEnabled := c.config.GetBool("autoscaling.workload.enabled")
		lazyStart := !podsRequiredAtStartup(c.config) && autoscalingEnabled

		if lazyStart {
			// The store is intentionally not added to objectStores. It would
			// block the startup readiness check.
			go c.startPodStoreOnGate(ctx, wlmetaStore, client, newPodStore)
		} else {
			reflector, store := newPodStore(ctx, wlmetaStore, c.config, client)
			objectStores = append(objectStores, store)
			go reflector.Run(ctx.Done())
			if autoscalingEnabled {
				go c.markPodCollectionSyncedWhenReady(ctx, store)
			}
		}
	}

	if shouldHaveDeploymentStore(c.config) {
		reflector, store := newDeploymentStore(ctx, wlmetaStore, c.config, client)
		objectStores = append(objectStores, store)
		go reflector.Run(ctx.Done())
	}

	if shouldHaveKueueMetadata(c.config) {
		gvrs, err := getGVRsForRequestedResources(client.Discovery(), kueueQueueGVRStrings())
		if err != nil {
			log.Errorf("failed to discover Kueue queue resources: %v", err)
		} else {
			for _, gvr := range gvrs {
				queueType, err := kubernetesresourceparsers.QueueTypeForKueueResource(gvr.Resource)
				if err != nil {
					log.Errorf("failed to get Kueue queue type for %s: %v", gvr.Resource, err)
					continue
				}
				reflector, store, err := newKueueQueueStore(ctx, wlmetaStore, apiserverClient.DynamicInformerCl, gvr, queueType)
				if err != nil {
					log.Errorf("failed to create Kueue queue store for %s: %v", gvr.Resource, err)
					continue
				}
				objectStores = append(objectStores, store)
				go reflector.Run(ctx.Done())
			}
		}

		gvrs, err = getGVRsForRequestedResources(client.Discovery(), kueueResourceFlavorGVRStrings())
		if err != nil {
			log.Errorf("failed to discover Kueue ResourceFlavor resources: %v", err)
		} else {
			for _, gvr := range gvrs {
				reflector, store, err := newKueueResourceFlavorStore(ctx, wlmetaStore, apiserverClient.DynamicInformerCl, gvr)
				if err != nil {
					log.Errorf("failed to create Kueue ResourceFlavor store for %s: %v", gvr.Resource, err)
					continue
				}
				objectStores = append(objectStores, store)
				go reflector.Run(ctx.Done())
			}
		}

		gvrs, err = getGVRsForRequestedResources(client.Discovery(), kueueWorkloadGVRStrings())
		if err != nil {
			log.Errorf("failed to discover Kueue Workload resources: %v", err)
		} else {
			for _, gvr := range gvrs {
				reflector, store, err := newKueueWorkloadStore(ctx, wlmetaStore, apiserverClient.DynamicInformerCl, gvr)
				if err != nil {
					log.Errorf("failed to create Kueue Workload store for %s: %v", gvr.Resource, err)
					continue
				}
				objectStores = append(objectStores, store)
				go reflector.Run(ctx.Done())
			}
		}
	}

	if c.config.GetBool("cluster_checks.crd_collection") {
		log.Info("Enabling CRD informer for workloadmeta collector")
		handlerRegistration, err := setupCRDInformer(wlmetaStore, apiserverClient.APIExentionsInformerFactory)
		if err != nil {
			log.Errorf("failed to setup CRD informer: %v", err)
		} else {
			log.Debug("CRD informer configured for workloadmeta")
			objectStores = append(objectStores, handlerRegistration)
		}
	}
	go collectKubeCapabilities(ctx, apiserverClient, wlmetaStore)

	go runStartupCheck(ctx, objectStores)

	return nil
}

// startPodStoreOnGate blocks until the autoscaling gate is enabled or the
// context is cancelled. On gate enable, it starts the pod reflector.
func (c *collector) startPodStoreOnGate(ctx context.Context, wlmetaStore workloadmeta.Component, client kubernetes.Interface, newStore storeGenerator) {
	if !c.autoscalingGate.WaitForEnable(ctx) {
		return
	}

	log.Debug("Autoscaling gate enabled, starting workloadmeta pod reflector lazily")
	reflector, store := newStore(ctx, wlmetaStore, c.config, client)
	go reflector.Run(ctx.Done())

	c.markPodCollectionSyncedWhenReady(ctx, store)
}

func (c *collector) markPodCollectionSyncedWhenReady(ctx context.Context, store *reflectorStore) {
	if !cache.WaitForCacheSync(ctx.Done(), store.HasSynced) {
		return
	}
	c.autoscalingGate.MarkPodCollectionSynced()
}

func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func collectKubeCapabilities(ctx context.Context, apiserverClient *apiserver.APIClient, wlmetaStore workloadmeta.Component) {
	featureGates, err := common.ClusterFeatureGates(ctx, apiserverClient.Cl.Discovery(), 15*time.Second)
	if err != nil {
		log.Infof("Couldn't collect cluster feature gates: %v", err)
		return
	}

	wlmFeatureGates := make(map[string]workloadmeta.FeatureGate)
	for name, featureGate := range featureGates {
		wlmFeatureGates[name] = workloadmeta.FeatureGate{
			Name:    featureGate.Name,
			Stage:   workloadmeta.FeatureGateStage(featureGate.Stage),
			Enabled: featureGate.Enabled,
		}
	}

	versionInfo, err := common.KubeServerVersion(apiserverClient.Cl.Discovery(), 15*time.Second)
	if err != nil {
		log.Infof("Couldn't collect cluster version: %v", err)
		return
	}

	wlmetaStore.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: &workloadmeta.KubeCapabilities{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubeCapabilities,
					ID:   workloadmeta.KubeCapabilitiesID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: workloadmeta.KubeCapabilitiesName,
				},
				Version:      versionInfo,
				FeatureGates: wlmFeatureGates,
			},
		},
	})
}

func runStartupCheck(ctx context.Context, stores []cache.ResourceEventHandlerRegistration) {
	log.Infof("Starting startup health check waiting for %d k8s reflectors to sync", len(stores))

	// There is no way to ensure liveness correctly as it would need to be plugged inside the
	// inner loop of Reflector.
	// However, we add Startup when we got at least some data.
	startupHealthCheck := health.RegisterReadiness(componentName, health.Once)

	// Checked synced, in its own scope to cleanly un-reference the syncTimer
	{
		syncTimer := time.NewTicker(time.Second)
	OUTER:
		for {
			select {
			case <-ctx.Done():
				break OUTER

			case <-syncTimer.C:
				allSynced := true
				for _, store := range stores {
					allSynced = allSynced && store.HasSynced()
				}

				if allSynced {
					break OUTER
				}
			}
		}
		syncTimer.Stop()
	}

	// Once synced, validate startup health check
	log.Infof("All (%d) K8S reflectors synced to workloadmeta", len(stores))
	<-startupHealthCheck.C
}

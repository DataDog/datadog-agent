// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

// Package kubemetadata implements the kube_metadata Workloadmeta collector.
package kubemetadata

import (
	"context"
	"time"

	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	collectorID          = "kube_metadata"
	componentName        = "workloadmeta-kube_metadata"
	namespaceMetadataTTL = 1 * time.Hour
)

type collector struct {
	id                          string
	store                       workloadmeta.Component
	catalog                     workloadmeta.AgentType
	seen                        map[workloadmeta.EntityID]struct{}
	namespaceLastSeen           map[string]time.Time
	kubeUtil                    kubelet.KubeUtilInterface
	apiClient                   *apiserver.APIClient
	dcaClient                   clusteragent.DCAClientInterface
	dcaEnabled                  bool
	updateFreq                  time.Duration
	lastUpdate                  time.Time
	collectNamespaceLabels      bool
	collectNamespaceAnnotations bool
	ignoreServiceReadiness      bool
	streaming                   *streamingProvider
}

// NewCollector returns a CollectorProvider to build a kubemetadata collector, and an error if any.
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:                collectorID,
			seen:              make(map[workloadmeta.EntityID]struct{}),
			namespaceLastSeen: make(map[string]time.Time),
			catalog:           workloadmeta.NodeAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

// Start tries to connect to the kubelet, the DCA and the API Server if the DCA is not available.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return errors.NewDisabled(componentName, "Agent is not running on Kubernetes")
	}

	c.store = store

	var err error
	c.kubeUtil, err = kubelet.GetKubeUtil()
	if err != nil {
		return err
	}

	// If DCA is enabled and can't communicate with the DCA, let worloadmeta retry.
	var errDCA error
	if pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") {
		c.dcaEnabled = false
		c.dcaClient, errDCA = clusteragent.GetClusterAgentClient()
		if errDCA != nil {
			log.Errorf("Could not initialise the communication with the cluster agent: %s", errDCA.Error())

			// Continue to retry while we can
			if retry.IsErrWillRetry(errDCA) {
				return errDCA
			}

			// We return the permanent fail only if fallback is disabled
			if retry.IsErrPermaFail(errDCA) && !pkgconfigsetup.Datadog().GetBool("cluster_agent.tagging_fallback") {
				return errDCA
			}

			log.Error("Permanent failure in communication with the cluster agent, will fallback to local service mapper")
		} else {
			c.dcaEnabled = true
		}
	}

	// Fallback to local metamapper if DCA not enabled, or in permafail state with fallback enabled.
	if !pkgconfigsetup.Datadog().GetBool("cluster_agent.enabled") || errDCA != nil {
		// Using GetAPIClient as error returned follows the IsErrWillRetry/IsErrPermaFail
		// Workloadmeta will retry calling this method until permafail
		c.apiClient, err = apiserver.GetAPIClient()
		if err != nil {
			return err
		}
	}

	c.updateFreq = time.Duration(pkgconfigsetup.Datadog().GetInt("kubernetes_metadata_tag_update_freq")) * time.Second

	metadataAsTags := configutils.GetMetadataAsTags(pkgconfigsetup.Datadog())
	c.collectNamespaceLabels = len(metadataAsTags.GetNamespaceLabelsAsTags()) > 0
	c.collectNamespaceAnnotations = len(metadataAsTags.GetNamespaceAnnotationsAsTags()) > 0
	c.ignoreServiceReadiness = pkgconfigsetup.Datadog().GetBool("kubernetes_kube_service_ignore_readiness")

	if c.dcaEnabled && pkgconfigsetup.Datadog().GetBool("kubernetes_metadata_streaming") {
		nodeName, nodeNameErr := c.kubeUtil.GetNodename(ctx)
		if nodeNameErr != nil {
			log.Warnf("Could not get node name, kube metadata streaming disabled: %v", nodeNameErr)
		} else {
			c.streaming = newStreamingProvider(
				nodeName,
				pkgconfigsetup.Datadog(),
				c.store,
				c.ignoreServiceReadiness,
				c.collectNamespaceLabels,
				c.collectNamespaceAnnotations,
			)
			c.streaming.start(ctx)
		}
	}

	return err
}

// Pull triggers an event collection from kubelet and the Datadog Cluster Agent.
// When streaming is active, this is a no-op.
func (c *collector) Pull(ctx context.Context) error {
	if c.streaming.isActive() {
		return nil
	}

	// Time constraints, get the delta in seconds to display it in the logs:
	timeDelta := c.lastUpdate.Add(c.updateFreq).Unix() - time.Now().Unix()
	if timeDelta > 0 {
		log.Tracef("skipping, next effective Pull will be in %d seconds", timeDelta)
		return nil
	}

	pods, err := c.kubeUtil.GetLocalPodList(ctx)
	if err != nil {
		return err
	}

	seen := make(map[workloadmeta.EntityID]struct{})
	events, err := c.parsePods(ctx, pods, seen)
	if err != nil {
		return err
	}

	// Create unset events for entities that are no longer seen
	for seenID := range c.seen {
		if _, ok := seen[seenID]; ok {
			continue
		}

		// If the entity is a namespace metadata entity and is within its TTL, skip creating an unset event.
		if namespaceName, isNsEntity := c.getNamespaceName(seenID); isNsEntity && c.shouldKeepNamespaceAlive(namespaceName) {
			continue
		}

		// Unset entities that are no longer seen
		events = append(events, createUnsetEvent(seenID))
	}

	c.seen = seen

	c.store.Notify(events)

	c.lastUpdate = time.Now()

	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

// createUnsetEvent creates an unset event for the appropriate entity type.
func createUnsetEvent(seenID workloadmeta.EntityID) workloadmeta.CollectorEvent {
	var entity workloadmeta.Entity
	switch seenID.Kind {
	case workloadmeta.KindKubernetesMetadata:
		entity = &workloadmeta.KubernetesMetadata{EntityID: seenID}
	case workloadmeta.KindKubernetesPod:
		entity = &workloadmeta.KubernetesPod{EntityID: seenID}
	default:
		log.Errorf("Unknown entity kind: %s", seenID.Kind)
	}

	return workloadmeta.CollectorEvent{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceClusterOrchestrator,
		Entity: entity,
	}
}

// getNamespaceName returns the namespace name if seenID is a namespace metadata entity
func (c *collector) getNamespaceName(seenID workloadmeta.EntityID) (string, bool) {
	if seenID.Kind != workloadmeta.KindKubernetesMetadata {
		return "", false
	}

	group, resource, _, name, err := util.ParseKubeMetadataEntityID(workloadmeta.KubeMetadataEntityID(seenID.ID))
	if err != nil || group != "" || resource != "namespaces" {
		return "", false
	}

	return name, true
}

// shouldKeepNamespaceAlive checks if a namespace is within its TTL.
// Returns false for unknown or expired namespaces (also cleans up tracking for expired ones).
func (c *collector) shouldKeepNamespaceAlive(namespaceName string) bool {
	lastSeen, ok := c.namespaceLastSeen[namespaceName]
	if !ok {
		return false
	}

	if time.Since(lastSeen) < namespaceMetadataTTL {
		return true
	}

	// Expired, remove from tracking
	delete(c.namespaceLastSeen, namespaceName)
	return false
}

// parsePods returns collection events based on a given podlist.
func (c *collector) parsePods(
	ctx context.Context,
	pods []*kubelet.Pod,
	seen map[workloadmeta.EntityID]struct{},
) ([]workloadmeta.CollectorEvent, error) {
	// selectPullBasedProvider is called on every pull (instead of Start)
	// because providers have a per-pull namespace cache.
	provider, err := c.selectPullBasedProvider(ctx)
	if err != nil {
		return nil, err
	}

	if err := provider.prepare(pods); err != nil {
		return nil, err
	}

	events := []workloadmeta.CollectorEvent{}

	for _, pod := range pods {
		if pod.Metadata.UID == "" {
			continue
		}

		services := []string{}
		if c.shouldCollectServices(pod) {
			if s := provider.getKubernetesServices(pod); s != nil {
				services = s
			}
		}

		nsLabels, nsAnnotations := provider.getNamespaceMetadata(pod.Metadata.Namespace)

		entityID := workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   pod.Metadata.UID,
		}

		seen[entityID] = struct{}{}

		entity := &workloadmeta.KubernetesPod{
			EntityID: entityID,
			EntityMeta: workloadmeta.EntityMeta{
				Name:        pod.Metadata.Name,
				Namespace:   pod.Metadata.Namespace,
				Annotations: pod.Metadata.Annotations,
				Labels:      pod.Metadata.Labels,
			},
			KubeServices:         services,
			NamespaceLabels:      nsLabels,
			NamespaceAnnotations: nsAnnotations,
		}

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceClusterOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: entity,
		})
	}

	// Save kubernetes namespace metadata entities for caching
	for ns, nsMetadata := range provider.getCollectedNamespaces() {
		nsEntity := createNamespaceEntity(ns, nsMetadata)
		nsEntityID := nsEntity.GetID()

		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceClusterOrchestrator,
			Type:   workloadmeta.EventTypeSet,
			Entity: nsEntity,
		})

		seen[nsEntityID] = struct{}{}
		c.namespaceLastSeen[ns] = time.Now()
	}

	return events, nil
}

func (c *collector) isDCAEnabled() bool {
	if c.dcaEnabled && c.dcaClient != nil {
		v := c.dcaClient.Version(false)
		if v.String() != "0.0.0" { // means not initialized
			return true
		}
	}

	return false
}

func (c *collector) selectPullBasedProvider(ctx context.Context) (metadataProvider, error) {
	if !c.isDCAEnabled() {
		return newLocalAPIServerProvider(c.apiClient), nil
	}

	dcaVersion := c.dcaClient.Version(false)
	if supportsPerNodePodMetadata(dcaVersion) {
		nodeName, err := c.kubeUtil.GetNodename(ctx)
		if err != nil {
			return nil, err
		}

		if c.dcaClient.SupportsNamespaceMetadataCollection() {
			return newDCAFullProvider(nodeName, c.dcaClient, c.collectNamespaceLabels, c.collectNamespaceAnnotations), nil
		}

		return newDCAPerNodeProvider(nodeName, c.dcaClient, c.collectNamespaceLabels, c.collectNamespaceAnnotations), nil
	}

	return newDCAPerPodProvider(c.dcaClient, c.collectNamespaceLabels, c.collectNamespaceAnnotations), nil
}

// createNamespaceEntity creates a KubernetesMetadata entity for a namespace
func createNamespaceEntity(namespaceName string, metadata namespaceMetadata) *workloadmeta.KubernetesMetadata {
	labels := make(map[string]string)
	annotations := make(map[string]string)

	if metadata.labels != nil {
		labels = metadata.labels
	}
	if metadata.annotations != nil {
		annotations = metadata.annotations
	}

	return &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(util.GenerateKubeMetadataEntityID("", "namespaces", "", namespaceName)),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        namespaceName,
			Labels:      labels,
			Annotations: annotations,
		},
		GVR: &schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "namespaces",
		},
	}
}

func (c *collector) shouldCollectServices(pod *kubelet.Pod) bool {
	return c.ignoreServiceReadiness || kubelet.IsPodReady(pod)
}

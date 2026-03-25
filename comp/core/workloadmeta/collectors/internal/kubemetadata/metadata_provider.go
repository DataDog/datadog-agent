// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// The collector supports multiple metadata providers because kube metadata can
// come from different sources depending on deployment mode and DCA version:
//
// - localAPIServerProvider: no DCA. The node Agent calls the kubernetes API server
// - dcaPerPodProvider:      DCA < 1.3, uses per-pod service mappings endpoint
// - dcaPerNodeProvider:     DCA >= 1.3, uses per-node service mappings endpoint
// - dcaFullProvider:        DCA >= 7.55, adds namespace metadata endpoint

// metadataProvider abstracts how pod services and namespace metadata are
// fetched.
type metadataProvider interface {
	// prepare is called once before iterating pods.
	prepare(pods []*kubelet.Pod) error
	// getKubernetesServices returns the kube services for a pod.
	getKubernetesServices(pod *kubelet.Pod) []string
	// getNamespaceMetadata returns namespace labels/annotations.
	getNamespaceMetadata(ns string) (labels, annotations map[string]string)
	// getCollectedNamespaces returns the namespaces collected while parsing pods.
	getCollectedNamespaces() map[string]namespaceMetadata
}

type namespaceMetadata struct {
	labels      map[string]string
	annotations map[string]string
}

type namespaceCache struct {
	collected map[string]namespaceMetadata
}

func newNamespaceCache() namespaceCache {
	return namespaceCache{
		collected: make(map[string]namespaceMetadata),
	}
}

func (c *namespaceCache) getCollectedNamespaces() map[string]namespaceMetadata {
	return c.collected
}

func (c *namespaceCache) get(ns string) (namespaceMetadata, bool) {
	metadata, found := c.collected[ns]
	return metadata, found
}

func (c *namespaceCache) set(ns string, metadata namespaceMetadata) namespaceMetadata {
	c.collected[ns] = metadata
	return metadata
}

// localAPIServerProvider is used when the node agent is not using the DCA and
// must resolve pod services from the local apiserver metadata mapper.
type localAPIServerProvider struct {
	apiClient *apiserver.APIClient
	nsCache   namespaceCache
}

func newLocalAPIServerProvider(apiClient *apiserver.APIClient) *localAPIServerProvider {
	return &localAPIServerProvider{
		apiClient: apiClient,
		nsCache:   newNamespaceCache(),
	}
}

func (p *localAPIServerProvider) prepare(pods []*kubelet.Pod) error {
	return addToCacheMetadataMapping(p.apiClient, pods)
}

func (p *localAPIServerProvider) getKubernetesServices(pod *kubelet.Pod) []string {
	metadata, err := apiserver.GetPodMetadataNames(pod.Spec.NodeName, pod.Metadata.Namespace, pod.Metadata.Name)
	if err != nil {
		log.Debugf("Could not fetch metadata for pod %s/%s: %v", pod.Metadata.Namespace, pod.Metadata.Name, err)
		return nil
	}

	return metadataNamesToServices(metadata)
}

func (p *localAPIServerProvider) getNamespaceMetadata(ns string) (labels, annotations map[string]string) {
	if _, found := p.nsCache.get(ns); !found {
		p.nsCache.set(ns, namespaceMetadata{})
	}
	return nil, nil
}

func (p *localAPIServerProvider) getCollectedNamespaces() map[string]namespaceMetadata {
	return p.nsCache.getCollectedNamespaces()
}

// dcaPerPodProvider is used with older DCA versions that do not expose the
// per-node metadata endpoint and use the per-pod endpoint instead.
type dcaPerPodProvider struct {
	dcaClient                   clusteragent.DCAClientInterface
	collectNamespaceLabels      bool
	collectNamespaceAnnotations bool
	nsCache                     namespaceCache
}

func newDCAPerPodProvider(
	dcaClient clusteragent.DCAClientInterface,
	collectNamespaceLabels bool,
	collectNamespaceAnnotations bool,
) *dcaPerPodProvider {
	return &dcaPerPodProvider{
		dcaClient:                   dcaClient,
		collectNamespaceLabels:      collectNamespaceLabels,
		collectNamespaceAnnotations: collectNamespaceAnnotations,
		nsCache:                     newNamespaceCache(),
	}
}

func (p *dcaPerPodProvider) prepare(_ []*kubelet.Pod) error {
	return nil
}

func (p *dcaPerPodProvider) getKubernetesServices(pod *kubelet.Pod) []string {
	metadata, err := p.dcaClient.GetKubernetesMetadataNames(pod.Spec.NodeName, pod.Metadata.Namespace, pod.Metadata.Name)
	if err != nil {
		log.Debugf("Could not fetch metadata for pod %s/%s: %v", pod.Metadata.Namespace, pod.Metadata.Name, err)
		return nil
	}

	return metadataNamesToServices(metadata)
}

func (p *dcaPerPodProvider) getNamespaceMetadata(ns string) (labels, annotations map[string]string) {
	return getNamespaceLabelsOnly(&p.nsCache, p.dcaClient, ns, p.collectNamespaceLabels, p.collectNamespaceAnnotations)
}

func (p *dcaPerPodProvider) getCollectedNamespaces() map[string]namespaceMetadata {
	return p.nsCache.getCollectedNamespaces()
}

// dcaPerNodeProvider is used with DCA versions that support per-node pod metadata
// but do not support the namespace metadata endpoint.
type dcaPerNodeProvider struct {
	nodeName                    string
	dcaClient                   clusteragent.DCAClientInterface
	collectNamespaceLabels      bool
	collectNamespaceAnnotations bool
	metadataByNsPods            apiv1.NamespacesPodsStringsSet
	nsCache                     namespaceCache
}

func newDCAPerNodeProvider(
	nodeName string,
	dcaClient clusteragent.DCAClientInterface,
	collectNamespaceLabels bool,
	collectNamespaceAnnotations bool,
) *dcaPerNodeProvider {
	return &dcaPerNodeProvider{
		nodeName:                    nodeName,
		dcaClient:                   dcaClient,
		collectNamespaceLabels:      collectNamespaceLabels,
		collectNamespaceAnnotations: collectNamespaceAnnotations,
		nsCache:                     newNamespaceCache(),
	}
}

func (p *dcaPerNodeProvider) prepare(_ []*kubelet.Pod) error {
	var err error
	p.metadataByNsPods, err = p.dcaClient.GetPodsMetadataForNode(p.nodeName)
	if err != nil {
		return fmt.Errorf("could not pull the metadata map of pods on node %s from the Datadog Cluster Agent: %w", p.nodeName, err)
	}

	return nil
}

func (p *dcaPerNodeProvider) getKubernetesServices(pod *kubelet.Pod) []string {
	if p.metadataByNsPods == nil {
		return nil
	}

	podsByNamespace, found := p.metadataByNsPods[pod.Metadata.Namespace]
	if !found {
		return nil
	}

	metadata, found := podsByNamespace[pod.Metadata.Name]
	if !found || metadata == nil {
		return nil
	}

	return metadataNamesToServices(sets.List(metadata))
}

func (p *dcaPerNodeProvider) getNamespaceMetadata(ns string) (labels, annotations map[string]string) {
	return getNamespaceLabelsOnly(&p.nsCache, p.dcaClient, ns, p.collectNamespaceLabels, p.collectNamespaceAnnotations)
}

func (p *dcaPerNodeProvider) getCollectedNamespaces() map[string]namespaceMetadata {
	return p.nsCache.getCollectedNamespaces()
}

// dcaFullProvider is used with DCA versions that support both per-node pod
// metadata and the namespace metadata endpoint for labels and annotations.
type dcaFullProvider struct {
	*dcaPerNodeProvider
}

func newDCAFullProvider(
	nodeName string,
	dcaClient clusteragent.DCAClientInterface,
	collectNamespaceLabels bool,
	collectNamespaceAnnotations bool,
) *dcaFullProvider {
	return &dcaFullProvider{
		dcaPerNodeProvider: newDCAPerNodeProvider(nodeName, dcaClient, collectNamespaceLabels, collectNamespaceAnnotations),
	}
}

func (p *dcaFullProvider) getNamespaceMetadata(ns string) (labels, annotations map[string]string) {
	if metadata, found := p.nsCache.get(ns); found {
		return selectNamespaceMetadata(metadata, p.collectNamespaceLabels, p.collectNamespaceAnnotations)
	}

	if !p.collectNamespaceLabels && !p.collectNamespaceAnnotations {
		p.nsCache.set(ns, namespaceMetadata{})
		return nil, nil
	}

	metadata, err := p.dcaClient.GetNamespaceMetadata(ns)
	if err != nil {
		log.Errorf("Could not fetch namespace metadata for namespace %s: %v", ns, err)
		return nil, nil
	}

	collected := p.nsCache.set(ns, fromClusterAgentMetadata(metadata))
	return selectNamespaceMetadata(collected, p.collectNamespaceLabels, p.collectNamespaceAnnotations)
}

func metadataNamesToServices(metadata []string) []string {
	services := make([]string, 0, len(metadata))
	for _, data := range metadata {
		parts := strings.Split(data, ":")
		switch len(parts) {
		case 1:
			services = append(services, parts[0])
		case 2:
			services = append(services, parts[1])
		}
	}
	return services
}

func selectNamespaceMetadata(metadata namespaceMetadata, includeLabels, includeAnnotations bool) (labels, annotations map[string]string) {
	if includeLabels {
		labels = metadata.labels
	}
	if includeAnnotations {
		annotations = metadata.annotations
	}

	return labels, annotations
}

func fromClusterAgentMetadata(metadata *clusteragent.Metadata) namespaceMetadata {
	if metadata == nil {
		return namespaceMetadata{}
	}

	return namespaceMetadata{
		labels:      metadata.Labels,
		annotations: metadata.Annotations,
	}
}

// getNamespaceLabelsOnly fetches namespace labels via the DCA labels-only
// endpoint.
func getNamespaceLabelsOnly(cache *namespaceCache, dcaClient clusteragent.DCAClientInterface, ns string, collectLabels, collectAnnotations bool) (map[string]string, map[string]string) {
	if metadata, found := cache.get(ns); found {
		return metadata.labels, nil
	}

	metadata := namespaceMetadata{}
	if collectLabels {
		var err error
		metadata.labels, err = dcaClient.GetNamespaceLabels(ns)
		if err != nil {
			log.Errorf("Could not fetch namespace labels for namespace %s: %v", ns, err)
			return nil, nil
		}
	}

	if collectAnnotations {
		log.Errorf("Could not fetch namespace annotations for namespace %s: kubernetes_namespace_annotations_as_tags requires version 7.55 or later of the cluster agent", ns)
	}

	cache.set(ns, metadata)
	return metadata.labels, nil
}

func addToCacheMetadataMapping(apiClient *apiserver.APIClient, kubeletPodList []*kubelet.Pod) error {
	if apiClient == nil {
		return nil
	}

	if len(kubeletPodList) == 0 {
		log.Debug("Empty kubelet pod list")
		return nil
	}

	reachablePods := make([]*kubelet.Pod, 0)
	nodeName := ""
	for _, p := range kubeletPodList {
		if p.Status.PodIP == "" {
			continue
		}
		if nodeName == "" && p.Spec.NodeName != "" {
			nodeName = p.Spec.NodeName
		}
		reachablePods = append(reachablePods, p)
	}

	return apiClient.NodeMetadataMapping(nodeName, reachablePods)
}

func supportsPerNodePodMetadata(dcaVersion version.Version) bool {
	return dcaVersion.Major > 1 || (dcaVersion.Major == 1 && dcaVersion.Minor >= 3)
}

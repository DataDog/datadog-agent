// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package apiserver

import (
	"context"

	v1 "k8s.io/api/core/v1"
	discv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// NodeMetadataMapping fetches endpoints or endpointslices from Kubernetes apiserver and adds the metadataMapper
// of the node to the cache
// Only called when the node agent computes the metadata mapper locally and does not rely on the DCA.
func (c *APIClient) NodeMetadataMapping(nodeName string, pods []*kubelet.Pod) error {
	var node v1.Node
	var nodeList v1.NodeList
	node.Name = nodeName
	nodeList.Items = append(nodeList.Items, node)

	if UseEndpointSlices() {
		sliceList, err := c.Cl.DiscoveryV1().EndpointSlices("").List(
			context.TODO(),
			metav1.ListOptions{
				TimeoutSeconds:  pointer.Ptr(int64(c.defaultClientTimeout.Seconds())),
				ResourceVersion: "0",
			},
		)
		if err != nil {
			log.Errorf("Could not collect endpointslices from the API Server: %q", err.Error())
			return err
		}
		if len(sliceList.Items) == 0 {
			log.Debug("No endpointslices collected from the API server")
			return nil
		}
		log.Debugf("Successfully collected %d endpointslices", len(sliceList.Items))

		processKubeServicesFromEndpointSlices(&nodeList, pods, sliceList.Items)
	} else {
		endpointList, err := c.Cl.CoreV1().Endpoints("").List(
			context.TODO(),
			metav1.ListOptions{
				TimeoutSeconds:  pointer.Ptr(int64(c.defaultClientTimeout.Seconds())),
				ResourceVersion: "0",
			},
		)
		if err != nil {
			log.Errorf("Could not collect endpoints from the API Server: %q", err.Error())
			return err
		}
		if endpointList.Items == nil {
			log.Debug("No endpoints collected from the API server")
			return nil
		}
		log.Debugf("Successfully collected %d endpoints", len(endpointList.Items))

		processKubeServices(&nodeList, pods, endpointList)
	}
	return nil
}

// processKubeServices adds services to the metadataMapper cache, pointer parameters must be non nil
func processKubeServices(nodeList *v1.NodeList, pods []*kubelet.Pod, endpointList *v1.EndpointsList) {
	if nodeList.Items == nil || len(pods) == 0 || endpointList.Items == nil {
		return
	}
	log.Debugf("Identified: %d node, %d pod, %d endpoints", len(nodeList.Items), len(pods), len(endpointList.Items))
	for _, node := range nodeList.Items {
		nodeName := node.Name
		nodeNameCacheKey := cache.BuildAgentKey(MetadataMapperCachePrefix, nodeName)
		newMetaBundle := NewMetadataMapperBundle()
		cacheMetadata(nodeName, nodeNameCacheKey, newMetaBundle, pods)

		if err := newMetaBundle.mapServices(nodeName, pods, *endpointList); err != nil {
			log.Errorf("Could not map the services on node %s: %s", node.Name, err.Error())
			continue
		}
		cache.Cache.Set(nodeNameCacheKey, newMetaBundle, metadataMapExpire)
	}
}

// processKubeServicesFromEndpointSlices adds services to the metadataMapper cache using EndpointSlices
func processKubeServicesFromEndpointSlices(nodeList *v1.NodeList, pods []*kubelet.Pod, slices []discv1.EndpointSlice) {
	if nodeList.Items == nil || len(pods) == 0 || len(slices) == 0 {
		return
	}
	log.Debugf("Identified: %d node, %d pod, %d endpointslices", len(nodeList.Items), len(pods), len(slices))
	for _, node := range nodeList.Items {
		nodeName := node.Name
		nodeNameCacheKey := cache.BuildAgentKey(MetadataMapperCachePrefix, nodeName)
		newMetaBundle := NewMetadataMapperBundle()
		cacheMetadata(nodeName, nodeNameCacheKey, newMetaBundle, pods)

		if err := newMetaBundle.mapServicesFromEndpointSlices(nodeName, pods, slices); err != nil {
			log.Errorf("Could not map the services on node %s: %s", node.Name, err.Error())
			continue
		}
		cache.Cache.Set(nodeNameCacheKey, newMetaBundle, metadataMapExpire)
	}
}

func cacheMetadata(nodeName string, cacheKey string, bundle *MetadataMapperBundle, pods []*kubelet.Pod) {
	freshness := cache.BuildAgentKey(MetadataMapperCachePrefix, nodeName, "freshness")

	cacheData, found := cache.Cache.Get(cacheKey)                // We get the old one with the dead pods. if diff reset metabundle and deleted key. Then compute again.
	freshnessCache, freshnessFound := cache.Cache.Get(freshness) // if expired, freshness not found deal with that

	if !found {
		cache.Cache.Set(freshness, len(pods), metadataMapExpire)
	}

	// We want to churn the cache every `metadataMapExpire` and if the number of entries varies between 2 runs..
	// If a pod is killed and rescheduled during a run, we will only keep the old entry for another run, which is acceptable.
	if found && freshnessCache != len(pods) || !freshnessFound {
		cache.Cache.Set(freshness, len(pods), metadataMapExpire)
		log.Debugf("Refreshing cache for %s", cacheKey)
	} else {
		oldMetadataBundle, ok := cacheData.(*MetadataMapperBundle)
		if ok {
			bundle.DeepCopy(oldMetadataBundle)
		}
	}
}

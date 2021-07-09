// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	"context"
	"fmt"
	"strings"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/simplelru"
)

// maxNamespacesInLRU limits the number of entries in the LRU cache for namespace labels on tags fetch
const maxNamespacesInLRU = 100

func (c *KubeMetadataCollector) getTagInfos(pods []*kubelet.Pod) []*TagInfo {
	var err error
	var metadataByNsPods apiv1.NamespacesPodsStringsSet
	if c.isClusterAgentEnabled() && c.dcaClient.Version().Major >= 1 && c.dcaClient.Version().Minor >= 3 {
		var nodeName string
		nodeName, err = c.kubeUtil.GetNodename(context.TODO())
		if err != nil {
			log.Errorf("Could not retrieve the Nodename, err: %v", err)
			return nil
		}
		metadataByNsPods, err = c.dcaClient.GetPodsMetadataForNode(nodeName)
		if err != nil {
			log.Debugf("Could not pull the metadata map of pods on node %s from the Datadog Cluster Agent: %s", nodeName, err.Error())
			return nil
		}
	}
	var tagInfo []*TagInfo
	var metadataNames []string
	var tag []string

	lruNamespaceTags, err := simplelru.NewLRU(maxNamespacesInLRU, nil)
	if err != nil {
		log.Debugf("Failed to create LRU for namespace tags: %v", err)
		return nil
	}

	for _, po := range pods {
		// Generate empty tagInfos for pods and their containers if pod
		// uses hostNetwork (we cannot define if a hostNetwork Pod is a
		// member of a service) or is not ready (it would not be an
		// endpoint of a service). This allows the tagger to cache the
		// result and avoid repeated calls to the DCA.
		if po.Spec.HostNetwork == true || !kubelet.IsPodReady(po) {
			for _, container := range po.Status.Containers {
				entityID, err := kubelet.KubeContainerIDToTaggerEntityID(container.ID)
				if err != nil {
					log.Warnf("Unable to parse container: %s", err)
					continue
				}

				tagInfo = append(tagInfo, &TagInfo{
					Source: kubeMetadataCollectorName,
					Entity: entityID,
				})
			}

			tagInfo = append(tagInfo, &TagInfo{
				Source: kubeMetadataCollectorName,
				Entity: kubelet.PodUIDToTaggerEntityName(po.Metadata.UID),
			})

			continue
		}

		var tagList *utils.TagList
		if c.hasNamespaceLabelsAsTags() {
			tagList = c.namespaceTagsFromLRU(lruNamespaceTags, po.Metadata.Namespace)
		}

		if tagList == nil {
			tagList = utils.NewTagList()
		}

		metadataNames, err = c.getMetadaNames(apiserver.GetPodMetadataNames, metadataByNsPods, po)
		if err != nil {
			log.Errorf("Could not fetch tags, %v", err)
		}
		for _, tagDCA := range metadataNames {
			log.Tracef("Tagging %s with %s", po.Metadata.Name, tagDCA)
			tag = strings.Split(tagDCA, ":")
			switch len(tag) {
			case 1:
				// c.dcaClient.GetPodsMetadataForNode returns only a list of services
				// but not the tag key
				tagList.AddLow("kube_service", tag[0])
			case 2:
				tagList.AddLow(tag[0], tag[1])
			default:
				continue
			}
		}

		low, orchestrator, high, _ := tagList.Compute()
		// Register the tags for the pod itself
		if po.Metadata.UID != "" {
			podInfo := &TagInfo{
				Source:               kubeMetadataCollectorName,
				Entity:               kubelet.PodUIDToTaggerEntityName(po.Metadata.UID),
				HighCardTags:         high,
				OrchestratorCardTags: orchestrator,
				LowCardTags:          low,
			}
			tagInfo = append(tagInfo, podInfo)
		}
		// Register the tags for all its containers
		for _, container := range po.Status.Containers {
			entityID, err := kubelet.KubeContainerIDToTaggerEntityID(container.ID)
			if err != nil {
				log.Warnf("Unable to parse container: %s", err)
				continue
			}
			info := &TagInfo{
				Source:               kubeMetadataCollectorName,
				Entity:               entityID,
				HighCardTags:         high,
				OrchestratorCardTags: orchestrator,
				LowCardTags:          low,
			}
			tagInfo = append(tagInfo, info)
		}
	}
	return tagInfo
}

func (c *KubeMetadataCollector) namespaceTagsFromLRU(lru *simplelru.LRU, ns string) *utils.TagList {
	var tagList *utils.TagList
	if v, ok := lru.Get(ns); ok {
		tagList = v.(*utils.TagList)
	} else {
		tagList = c.getNamespaceTags(apiserver.GetNamespaceLabels, ns)
		lru.Add(ns, tagList)
	}
	if tagList != nil {
		tagList = tagList.Copy()
	}
	return tagList
}

func (c *KubeMetadataCollector) getMetadaNames(getPodMetaDataFromAPIServerFunc func(string, string, string) ([]string, error), metadataByNsPods apiv1.NamespacesPodsStringsSet, po *kubelet.Pod) ([]string, error) {
	if !c.isClusterAgentEnabled() {
		metadataNames, err := getPodMetaDataFromAPIServerFunc(po.Spec.NodeName, po.Metadata.Namespace, po.Metadata.Name)
		if err != nil {
			err = fmt.Errorf("Could not fetch cluster level tags of pod: %s, %v", po.Metadata.Name, err)
		}
		return metadataNames, err
	}

	if metadataByNsPods != nil {
		if data, ok := metadataByNsPods[po.Metadata.Namespace][po.Metadata.Name]; ok && data != nil {
			return data.List(), nil
		}
		return nil, nil
	}

	metadataNames, err := c.dcaClient.GetKubernetesMetadataNames(po.Spec.NodeName, po.Metadata.Namespace, po.Metadata.Name)
	if err != nil {
		err = fmt.Errorf("Could not pull the metadata map of pod %s on node %s, %v", po.Metadata.Name, po.Spec.NodeName, err)
	}
	return metadataNames, err
}

func (c *KubeMetadataCollector) getNamespaceTags(getNamespaceLabelsFromAPIServerFunc func(string) (map[string]string, error), ns string) *utils.TagList {
	if !c.hasNamespaceLabelsAsTags() {
		return nil
	}

	if c.isClusterAgentEnabled() {
		getNamespaceLabelsFromAPIServerFunc = c.dcaClient.GetNamespaceLabels
	}
	labels, err := getNamespaceLabelsFromAPIServerFunc(ns)
	if err != nil {
		_ = log.Warnf("Could not fetch labels for namespace: %s, %v", ns, err)
		return nil
	}

	tags := utils.NewTagList()
	for name, value := range labels {
		utils.AddMetadataAsTags(name, value, c.namespaceLabelsAsTags, c.globNamespaceLabels, tags)
	}
	return tags
}

// addToCacheMetadataMapping is acting like the DCA at the node level.
func (c *KubeMetadataCollector) addToCacheMetadataMapping(kubeletPodList []*kubelet.Pod) error {
	if len(kubeletPodList) == 0 {
		log.Debugf("Empty kubelet pod list")
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
	return c.apiClient.NodeMetadataMapping(nodeName, reachablePods)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	"fmt"
	"strings"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *KubeMetadataCollector) getTagInfos(pods []*kubelet.Pod) []*TagInfo {
	var err error
	var metadataByNsPods apiv1.NamespacesPodsStringsSet
	if c.isClusterAgentEnabled() && c.dcaClient.Version().Major >= 1 && c.dcaClient.Version().Minor >= 3 {
		var nodeName string
		nodeName, err = c.kubeUtil.GetNodename()
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
	for _, po := range pods {
		if kubelet.IsPodReady(po) == false {
			log.Debugf("pod %q is not ready, skipping", po.Metadata.Name)
			continue
		}

		// We cannot define if a hostNetwork Pod is a member of a service
		if po.Spec.HostNetwork == true {
			for _, container := range po.Status.Containers {
				entityID, err := kubelet.KubeContainerIDToEntityID(container.ID)
				if err != nil {
					log.Warnf("Unable to parse container: %s", err)
					continue
				}
				info := &TagInfo{
					Source:               kubeMetadataCollectorName,
					Entity:               entityID,
					HighCardTags:         []string{},
					OrchestratorCardTags: []string{},
					LowCardTags:          []string{},
				}
				tagInfo = append(tagInfo, info)
			}
			continue
		}

		tagList := utils.NewTagList()
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

		low, orchestrator, high := tagList.Compute()
		// Register the tags for the pod itself
		if po.Metadata.UID != "" {
			podInfo := &TagInfo{
				Source:               kubeMetadataCollectorName,
				Entity:               kubelet.PodUIDToEntityName(po.Metadata.UID),
				HighCardTags:         high,
				OrchestratorCardTags: orchestrator,
				LowCardTags:          low,
			}
			tagInfo = append(tagInfo, podInfo)
		}
		// Register the tags for all its containers
		for _, container := range po.Status.Containers {
			entityID, err := kubelet.KubeContainerIDToEntityID(container.ID)
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

func (c *KubeMetadataCollector) getMetadaNames(getPodMetaDataFromApiServerFunc func(string, string, string) ([]string, error), metadataByNsPods apiv1.NamespacesPodsStringsSet, po *kubelet.Pod) ([]string, error) {
	if !c.isClusterAgentEnabled() {
		metadataNames, err := getPodMetaDataFromApiServerFunc(po.Spec.NodeName, po.Metadata.Namespace, po.Metadata.Name)
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

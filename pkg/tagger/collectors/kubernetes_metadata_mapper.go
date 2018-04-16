// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	log "github.com/cihub/seelog"

	"github.com/ericchiang/k8s/api/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"strings"
)

func (c *KubeMetadataCollector) getTagInfos(pods []*kubelet.Pod) []*TagInfo {
	var tagInfo []*TagInfo
	var metadataNames []string
	var tag []string
	var err error
	for _, po := range pods {
		if kubelet.IsPodReady(po) == false {
			log.Debugf("pod %q is not ready, skipping", po.Metadata.Name)
			continue
		}

		// We cannot define if a hostNetwork Pod is a member of a service
		if po.Spec.HostNetwork == true {
			for _, container := range po.Status.Containers {
				info := &TagInfo{
					Source:       kubeMetadataCollectorName,
					Entity:       container.ID,
					HighCardTags: []string{},
					LowCardTags:  []string{},
				}
				tagInfo = append(tagInfo, info)
			}
			continue
		}

		tagList := utils.NewTagList()
		if !config.Datadog.GetBool("cluster_agent") {
			metadataNames, err = apiserver.GetPodMetadataNames(po.Spec.NodeName, po.Metadata.Name)
			if err != nil {
				log.Errorf("Could not fetch tags for pod %s from the Cluster Agent: %s", po.Metadata.Name, err.Error())
				continue
			}
		} else {
			metadataNames, err = c.dcaClient.GetKubernetesMetadataNames(po.Spec.NodeName, po.Metadata.Name)
			if err != nil {
				log.Tracef("Could not pull the metadata map of po %s on node %s from the Datadog Cluster Agent: %s", po.Metadata.Name, po.Spec.NodeName, err.Error())
			}
		}
		for _, tagDCA := range metadataNames {
			// for service in metadataName.[services]
			log.Tracef("Tagging %s with %s", po.Metadata.Name, tagDCA)
			tag = strings.Split(tagDCA, ":")
			if len(tag) != 2 {
				continue
			}
			tagList.AddLow(tag[0], tag[1])
		}

		low, high := tagList.Compute()
		for _, container := range po.Status.Containers {
			info := &TagInfo{
				Source:       kubeMetadataCollectorName,
				Entity:       container.ID,
				HighCardTags: high,
				LowCardTags:  low,
			}
			tagInfo = append(tagInfo, info)
		}
	}
	return tagInfo
}

// addToCacheMetadataMapping is acting like the DCA at the node level.
func (c *KubeMetadataCollector) addToCacheMetadataMapping(kubeletPodList []*kubelet.Pod) error {
	if len(kubeletPodList) == 0 {
		log.Debugf("Empty kubelet pod list")
		return nil
	}

	podList := &v1.PodList{}
	nodeName := ""
	for _, p := range kubeletPodList {
		if p.Status.PodIP == "" {
			continue
		}
		if nodeName == "" && p.Spec.NodeName != "" {
			nodeName = p.Spec.NodeName
		}
		pod := &v1.Pod{
			Metadata: &metav1.ObjectMeta{
				Name:      &p.Metadata.Name,
				Namespace: &p.Metadata.Namespace,
			},
			Status: &v1.PodStatus{
				PodIP: &p.Status.PodIP,
			},
		}
		podList.Items = append(podList.Items, pod)
	}
	return c.apiClient.NodeMetadataMapping(nodeName, podList)
}

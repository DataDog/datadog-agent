// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver,kubelet

package collectors

import (
	"strings"

	log "github.com/cihub/seelog"

	"github.com/ericchiang/k8s/api/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

/*
TODO remove this file when we have the DCA
*/

func (c *KubeServiceCollector) getTagInfos(pods []*kubelet.Pod) []*TagInfo {
	var tagInfo []*TagInfo
	var serviceNames []string
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
					Source:       kubeServiceCollectorName,
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
			serviceNames, err = apiserver.GetPodServiceNames(po.Spec.NodeName, po.Metadata.Name)
			if err != nil {
				log.Error("Could not fetch tags for pod %s from the Cluster Agent: %s", po.Metadata.Name, err.Error())
				continue
			}
		} else {
			serviceNames, err = c.dcaClient.GetKubernetesServiceNames(po.Spec.NodeName, po.Metadata.Name)
			if err != nil {
				log.Tracef("Could not pull the service map of po %s on node %s from the Datadog Cluster Agent: %s", po.Spec.NodeName, po.Metadata.Name, err.Error())
			}

		}
		log.Debugf("nodeName: %s, podName: %s, services: %q", po.Spec.NodeName, po.Metadata.Name, strings.Join(serviceNames, ","))
		for _, serviceName := range serviceNames {
			log.Tracef("tagging %s kube_service:%s", po.Metadata.Name, serviceName)
			tagList.AddLow("kube_service", serviceName)
		}

		low, high := tagList.Compute()
		for _, container := range po.Status.Containers {
			info := &TagInfo{
				Source:       kubeServiceCollectorName,
				Entity:       container.ID,
				HighCardTags: high,
				LowCardTags:  low,
			}
			tagInfo = append(tagInfo, info)
		}
	}
	return tagInfo
}

// addToCacheServiceMapping is acting like the DCA at the node level.
func (c *KubeServiceCollector) addToCacheServiceMapping(kubeletPodList []*kubelet.Pod) error {
	if len(kubeletPodList) == 0 {
		log.Debugf("empty kubelet pod list")
		return nil
	}

	log.Debugf("refreshing the service mapping...")
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
	return c.apiClient.NodeServiceMapping(nodeName, podList)
}

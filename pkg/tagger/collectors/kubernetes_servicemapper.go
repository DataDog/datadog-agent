// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package collectors

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/ericchiang/k8s/api/v1"
	metav1 "github.com/ericchiang/k8s/apis/meta/v1"
)

// doServiceMapping TODO refactor when we have the DCA
func doServiceMapping(kubeletPodList []*kubelet.Pod) {
	if len(kubeletPodList) == 0 {
		log.Debugf("empty kubelet pod list")
		return
	}

	apiC, err := apiserver.GetAPIClient()
	if err != nil {
		return
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
	apiC.NodeServiceMapping(nodeName, podList)
}

// doServiceMapping TODO refactor when we have the DCA
func getPodServiceNames(nodeName, podName string) []string {
	log.Debugf("getting %s/%s", nodeName, podName)
	return apiserver.GetPodServiceNames(nodeName, podName)
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"regexp"

	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type podParser struct {
	annotationsFilter []*regexp.Regexp
}

// NewPodParser creates and returns a pod parser based on annotations exclusion list
func NewPodParser(annotationsExclude []string) (ObjectParser, error) {
	filters, err := parseFilters(annotationsExclude)
	if err != nil {
		return nil, err
	}

	return podParser{annotationsFilter: filters}, nil
}

func (p podParser) Parse(obj interface{}) workloadmeta.Entity {
	pod := obj.(*corev1.Pod)
	owners := make([]workloadmeta.KubernetesPodOwner, 0, len(pod.OwnerReferences))
	for _, o := range pod.OwnerReferences {
		owners = append(owners, workloadmeta.KubernetesPodOwner{
			Kind: o.Kind,
			Name: o.Name,
			ID:   string(o.UID),
		})
	}

	var pvcNames []string
	for _, volume := range pod.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			pvcNames = append(pvcNames, volume.PersistentVolumeClaim.ClaimName)
		}
	}

	var rtcName string
	if pod.Spec.RuntimeClassName != nil {
		rtcName = *pod.Spec.RuntimeClassName
	}

	var gpuVendorList []string
	uniqueGPUVendor := make(map[string]struct{})
	for _, container := range pod.Spec.Containers {
		for resourceName := range container.Resources.Limits {
			gpuName, found := gpu.ExtractSimpleGPUName(gpu.ResourceGPU(resourceName))
			if found {
				uniqueGPUVendor[gpuName] = struct{}{}
			}
		}
	}
	for gpuVendor := range uniqueGPUVendor {
		gpuVendorList = append(gpuVendorList, gpuVendor)
	}

	containersList := make([]workloadmeta.OrchestratorContainer, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		c := workloadmeta.OrchestratorContainer{
			Name: container.Name,
		}
		if cpuReq, found := container.Resources.Requests[corev1.ResourceCPU]; found {
			c.Resources.CPURequest = kubernetes.FormatCPURequests(cpuReq)
		}
		if memoryReq, found := container.Resources.Requests[corev1.ResourceMemory]; found {
			c.Resources.MemoryRequest = kubernetes.FormatMemoryRequests(memoryReq)
		}
		containersList = append(containersList, c)
	}

	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   string(pod.UID),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Annotations: filterMapStringKey(pod.Annotations, p.annotationsFilter),
			Labels:      pod.Labels,
		},
		Phase:                      string(pod.Status.Phase),
		Owners:                     owners,
		PersistentVolumeClaimNames: pvcNames,
		Ready:                      isPodReady(pod),
		IP:                         pod.Status.PodIP,
		PriorityClass:              pod.Spec.PriorityClassName,
		QOSClass:                   string(pod.Status.QOSClass),
		RuntimeClass:               rtcName,
		GPUVendorList:              gpuVendorList,
		Containers:                 containersList,
	}
}

// Should be aligned with pkg/util/kubernetes/kubelet/kubelet.go
// (Except the static pods that should go away as fixed long time ago)
func isPodReady(pod *corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

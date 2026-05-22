// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// computePodQoSFromSpec mirrors Kubernetes' QoS classification
// (k8s.io/kubernetes/pkg/apis/core/v1/helper/qos.GetPodQOS) so it can be evaluated
// at admission time before the kubelet has set pod.Status.QOSClass. Only CPU and
// memory contribute to the classification.
//
// All containers — regular containers, init containers, and sidecar init containers
// (restartPolicy=Always) — participate. The classification rules are:
//   - BestEffort: no positive CPU or memory requests/limits anywhere.
//   - Guaranteed: every contributing container has both CPU and memory limits set
//     (positive), and per-resource the summed requests equal the summed limits.
//   - Burstable: anything else.
func computePodQoSFromSpec(spec *corev1.PodSpec) corev1.PodQOSClass {
	if spec == nil {
		return corev1.PodQOSBestEffort
	}

	requests := corev1.ResourceList{}
	limits := corev1.ResourceList{}
	isGuaranteed := true

	containers := make([]*corev1.Container, 0, len(spec.Containers)+len(spec.InitContainers))
	for i := range spec.Containers {
		containers = append(containers, &spec.Containers[i])
	}
	for i := range spec.InitContainers {
		containers = append(containers, &spec.InitContainers[i])
	}

	for _, c := range containers {
		for name, qty := range c.Resources.Requests {
			if !isQoSResource(name) || qty.Sign() <= 0 {
				continue
			}
			addToResourceList(requests, name, qty)
		}

		hasCPULimit, hasMemLimit := false, false
		for name, qty := range c.Resources.Limits {
			if !isQoSResource(name) || qty.Sign() <= 0 {
				continue
			}
			switch name {
			case corev1.ResourceCPU:
				hasCPULimit = true
			case corev1.ResourceMemory:
				hasMemLimit = true
			}
			addToResourceList(limits, name, qty)
		}
		if !hasCPULimit || !hasMemLimit {
			isGuaranteed = false
		}
	}

	if len(requests) == 0 && len(limits) == 0 {
		return corev1.PodQOSBestEffort
	}

	if isGuaranteed && len(requests) == len(limits) {
		for name, req := range requests {
			if lim, ok := limits[name]; !ok || lim.Cmp(req) != 0 {
				isGuaranteed = false
				break
			}
		}
		if isGuaranteed {
			return corev1.PodQOSGuaranteed
		}
	}
	return corev1.PodQOSBurstable
}

func isQoSResource(name corev1.ResourceName) bool {
	return name == corev1.ResourceCPU || name == corev1.ResourceMemory
}

func addToResourceList(rl corev1.ResourceList, name corev1.ResourceName, qty resource.Quantity) {
	if existing, ok := rl[name]; ok {
		existing.Add(qty)
		rl[name] = existing
		return
	}
	rl[name] = qty.DeepCopy()
}

// podIsGuaranteedFromSpec returns true when the pod-spec-computed QoS class is Guaranteed.
// Used by the admission webhook before pod.Status.QOSClass is populated by the kubelet.
func podIsGuaranteedFromSpec(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	return computePodQoSFromSpec(&pod.Spec) == corev1.PodQOSGuaranteed
}

// podIsGuaranteedInPlace returns true when the kubelet-reported QoS class on a running pod
// (as collected by workloadmeta) is Guaranteed. Used by the in-place resize path where the
// pod has already been admitted and scheduled.
func podIsGuaranteedInPlace(pod *workloadmeta.KubernetesPod) bool {
	if pod == nil {
		return false
	}
	return pod.QOSClass == string(corev1.PodQOSGuaranteed)
}

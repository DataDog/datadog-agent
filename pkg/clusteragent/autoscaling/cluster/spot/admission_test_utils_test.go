// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package spot contains logic to schedule pods on spot instances.
package spot

import corev1 "k8s.io/api/core/v1"

// PodCreated adapts the existing simulation to the decision API.
func (s *scheduler) PodCreated(pod *corev1.Pod) (bool, error) {
	placement, err := s.PlanPlacement(pod)
	if err != nil || placement == nil {
		return false, err
	}
	assignToSpot(pod)
	return true, nil
}

func assignToSpot(pod *corev1.Pod) {
	if pod.Spec.NodeSelector == nil {
		pod.Spec.NodeSelector = map[string]string{}
	}
	pod.Spec.NodeSelector[spotNodeLabelKey] = spotNodeLabelValue
	pod.Spec.Tolerations = append(pod.Spec.Tolerations, corev1.Toleration{
		Key:      spotNodeTaintKey,
		Operator: corev1.TolerationOpEqual,
		Value:    spotNodeTaintValue,
		Effect:   corev1.TaintEffectNoSchedule,
	})

	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[SpotAssignedLabel] = SpotAssignedLabelValue
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package spot contains logic to schedule pods on spot instances.
package spot

import corev1 "k8s.io/api/core/v1"

// PodHandler handles pod admission events for spot scheduling.
type PodHandler interface {
	// PlanPlacement is called when a pod is created via admission webhook.
	// It returns scheduling intent, or nil when the pod is left unchanged.
	PlanPlacement(pod *corev1.Pod) (*PodPlacement, error)
	// PodDeleted is called when a pod is deleted via admission webhook.
	PodDeleted(pod *corev1.Pod)
}

// PodPlacement describes intended scheduling edits after one tracker decision.
// It contains no admission transport or patch mechanics.
type PodPlacement struct {
	NodeSelector map[string]string
	Labels       map[string]string
	Tolerations  []corev1.Toleration
}

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
	// PodCreated is called when a pod is created via admission webhook.
	// It returns true if the pod was mutated to target a spot instance.
	PodCreated(pod *corev1.Pod) (bool, error)
	// PodDeleted is called when a pod is deleted via admission webhook.
	PodDeleted(pod *corev1.Pod)
}

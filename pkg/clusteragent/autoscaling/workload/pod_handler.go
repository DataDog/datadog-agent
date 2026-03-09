// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"errors"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/spot"
)

// PodHandler handles [corev1.Pod] creation and deletion requests.
type PodHandler struct {
	podPatcher    PodPatcher
	spotScheduler *spot.Scheduler // could be nil
}

// NewPodHandler creates PodHandler that delegates [corev1.Pod] creation and deletion requests to
// podPatcher and optionally to spotScheduler if it is not nil.
func NewPodHandler(podPatcher PodPatcher, spotScheduler *spot.Scheduler) *PodHandler {
	return &PodHandler{podPatcher: podPatcher, spotScheduler: spotScheduler}
}

func (h *PodHandler) PodCreated(pod *corev1.Pod) (bool, error) {
	patcherUpdated, patcherErr := h.podPatcher.ApplyRecommendations(pod)
	if h.spotScheduler == nil {
		return patcherUpdated, patcherErr
	}
	spotUpdated, spotErr := h.spotScheduler.PodCreated(pod)
	return patcherUpdated || spotUpdated, errors.Join(patcherErr, spotErr)
}

func (h *PodHandler) PodDeleted(pod *corev1.Pod) {
	if h.spotScheduler == nil {
		return
	}
	h.spotScheduler.PodDeleted(pod)
}

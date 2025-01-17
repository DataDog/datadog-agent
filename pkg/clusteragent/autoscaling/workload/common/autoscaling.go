// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"context"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// ScaleDirection represents the scaling direction
type ScaleDirection string

const (
	// NoScale indicates no scaling action is needed
	NoScale ScaleDirection = "noScale"
	// ScaleUp indicates scaling up is needed
	ScaleUp ScaleDirection = "scaleUp"
	// ScaleDown indicates scaling down is needed
	ScaleDown ScaleDirection = "scaleDown"
)

// NamespacedPodOwner represents a pod owner in a namespace
type NamespacedPodOwner struct {
	// Namespace is the namespace of the pod owner
	Namespace string
	// Kind is the kind of the pod owner (e.g. Deployment, StatefulSet etc.)
	// ReplicaSet is replaced by Deployment
	Kind string
	// Name is the name of the pod owner
	Name string
}

// PodWatcher indexes pods by their owner
type PodWatcher interface {
	// Start starts the PodWatcher.
	Run(ctx context.Context)
	// GetPodsForOwner returns the pods for the given owner.
	GetPodsForOwner(NamespacedPodOwner) []*workloadmeta.KubernetesPod
}

// GetScaleDirection gets the scaling direction based on the current number of replicas vs the recommendation
func GetScaleDirection(currentReplicas, recommendedReplicas int32) ScaleDirection {
	if currentReplicas < recommendedReplicas {
		return ScaleUp
	} else if currentReplicas > recommendedReplicas {
		return ScaleDown
	}
	return NoScale
}

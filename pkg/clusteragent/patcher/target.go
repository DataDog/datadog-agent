// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package patcher provides a shared API for patching Kubernetes workload
// resources (Deployments, StatefulSets, Pods, Argo Rollouts, etc.) from the
// Cluster Agent. It unifies the patch construction, Kubernetes API interaction,
// and leader-guard patterns used across language detection, remote config,
// autoscaling, and other subsystems.
package patcher

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Target identifies a Kubernetes resource to patch.
type Target struct {
	// GVR is the GroupVersionResource of the target.
	GVR schema.GroupVersionResource
	// Namespace of the target resource. Empty for cluster-scoped resources.
	Namespace string
	// Name of the target resource.
	Name string
}

func (t Target) String() string {
	return fmt.Sprintf("%s/%s/%s", t.GVR.Resource, t.Namespace, t.Name)
}

var (
	deploymentGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	statefulSetGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	podGVR         = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	rolloutGVR     = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"}
)

// DeploymentTarget creates a Target for a namespaced Deployment.
func DeploymentTarget(namespace, name string) Target {
	return Target{GVR: deploymentGVR, Namespace: namespace, Name: name}
}

// StatefulSetTarget creates a Target for a namespaced StatefulSet.
func StatefulSetTarget(namespace, name string) Target {
	return Target{GVR: statefulSetGVR, Namespace: namespace, Name: name}
}

// PodTarget creates a Target for a namespaced Pod.
func PodTarget(namespace, name string) Target {
	return Target{GVR: podGVR, Namespace: namespace, Name: name}
}

// RolloutTarget creates a Target for a namespaced Argo Rollout.
func RolloutTarget(namespace, name string) Target {
	return Target{GVR: rolloutGVR, Namespace: namespace, Name: name}
}

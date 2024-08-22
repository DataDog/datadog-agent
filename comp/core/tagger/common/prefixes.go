// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package common provides common constants and methods for the tagger component and implementation
package common

import "github.com/DataDog/datadog-agent/comp/core/tagger/types"

const (
	// ContainerID is the prefix `container_id`
	ContainerID types.EntityIDPrefix = "container_id"
	// ContainerImageMetadata is the prefix `container_image_metadata`
	ContainerImageMetadata types.EntityIDPrefix = "container_image_metadata"
	// ECSTask is the prefix `ecs_task`
	ECSTask types.EntityIDPrefix = "ecs_task"
	// Host is the prefix `host`
	Host types.EntityIDPrefix = "host"
	// KubernetesDeployment is the prefix `deployment`
	KubernetesDeployment types.EntityIDPrefix = "deployment"
	// KubernetesMetadata is the prefix `kubernetes_metadata`
	KubernetesMetadata types.EntityIDPrefix = "kubernetes_metadata"
	// KubernetesPodUID is the prefix `kubernetes_pod_uid`
	KubernetesPodUID types.EntityIDPrefix = "kubernetes_pod_uid"
	// Process is the prefix `process`
	Process types.EntityIDPrefix = "process"
)

// AllPrefixesSet returns a set of all possible entity id prefixes that can be used in the tagger
func AllPrefixesSet() map[types.EntityIDPrefix]struct{} {
	return map[types.EntityIDPrefix]struct{}{
		ContainerID:            {},
		ContainerImageMetadata: {},
		ECSTask:                {},
		Host:                   {},
		KubernetesDeployment:   {},
		KubernetesMetadata:     {},
		KubernetesPodUID:       {},
		Process:                {},
	}
}

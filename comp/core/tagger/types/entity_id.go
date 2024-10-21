// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

const separator = "://"
const separatorLength = len(separator)

// GetSeparatorLengh returns the length of the entityID separator
func GetSeparatorLengh() int {
	return separatorLength
}

// EntityID represents a tagger entityID
type EntityID struct {
	prefix EntityIDPrefix
	id     string
}

// Empty returns true if prefix and id are both empty strings
func (eid EntityID) Empty() bool {
	return eid.prefix == "" && eid.id == ""
}

// GetPrefix returns the entityID prefix
func (eid EntityID) GetPrefix() EntityIDPrefix {
	return eid.prefix
}

// GetID returns the id excluding the prefix
func (eid EntityID) GetID() string {
	return eid.id
}

// String returns the entityID in the format `{prefix}://{id}`
func (eid EntityID) String() string {
	return eid.prefix.ToUID(eid.id)
}

// NewEntityID builds and returns an EntityID object
func NewEntityID(prefix EntityIDPrefix, id string) EntityID {
	return EntityID{prefix, id}
}

const (
	// ContainerID is the prefix `container_id`
	ContainerID EntityIDPrefix = "container_id"
	// ContainerImageMetadata is the prefix `container_image_metadata`
	ContainerImageMetadata EntityIDPrefix = "container_image_metadata"
	// ECSTask is the prefix `ecs_task`
	ECSTask EntityIDPrefix = "ecs_task"
	// Host is the prefix `host`
	Host EntityIDPrefix = "host"
	// KubernetesDeployment is the prefix `deployment`
	KubernetesDeployment EntityIDPrefix = "deployment"
	// KubernetesMetadata is the prefix `kubernetes_metadata`
	KubernetesMetadata EntityIDPrefix = "kubernetes_metadata"
	// KubernetesPodUID is the prefix `kubernetes_pod_uid`
	KubernetesPodUID EntityIDPrefix = "kubernetes_pod_uid"
	// Process is the prefix `process`
	Process EntityIDPrefix = "process"
)

// AllPrefixesSet returns a set of all possible entity id prefixes that can be used in the tagger
func AllPrefixesSet() map[EntityIDPrefix]struct{} {
	return map[EntityIDPrefix]struct{}{
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"fmt"
	"strings"
)

const separator = "://"
const separatorLength = len(separator)

var globalEntityID = NewEntityID(InternalID, "global-entity-id")

// GetSeparatorLength returns the length of the entityID separator
func GetSeparatorLength() int {
	return separatorLength
}

// EntityID represents a tagger entityID
type EntityID struct {
	prefix EntityIDPrefix
	id     string
}

// Empty returns true if either the prefix or id empty strings
func (eid EntityID) Empty() bool {
	return eid.prefix == "" || eid.id == ""
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
// Returns an empty string if `id` is empty
func (eid EntityID) String() string {
	return eid.prefix.ToUID(eid.id)
}

var supportedPrefixes = AllPrefixesSet()

// NewEntityID builds and returns an EntityID object
// A panic will occur if an unsupported prefix is used
func NewEntityID(prefix EntityIDPrefix, id string) EntityID {
	if _, found := supportedPrefixes[prefix]; !found {
		// prefix is expected to be set based on the prefix enum defined below
		panic(fmt.Sprintf("unsupported tagger entity prefix: %q", prefix))
	}
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
	// InternalID is the prefix `internal`
	InternalID EntityIDPrefix = "internal"
	// GPU is the prefix `gpu`
	GPU EntityIDPrefix = "gpu"
	// Kubelet is the prefix `kubelet`
	Kubelet EntityIDPrefix = "kubelet"
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
		InternalID:             {},
		GPU:                    {},
		Kubelet:                {},
	}
}

// GetGlobalEntityID returns the entity ID that holds global tags
func GetGlobalEntityID() EntityID {
	return globalEntityID
}

// ExtractPrefixAndID extracts prefix and id from tagger entity id and returns an error if the received entityID is not valid
func ExtractPrefixAndID(entityID string) (prefix EntityIDPrefix, id string, err error) {
	extractedPrefix, extractedID, found := strings.Cut(entityID, "://")
	if !found {
		return "", "", fmt.Errorf("unsupported tagger entity id format %q, correct format is `{prefix}://{id}`", entityID)
	}
	if _, found := supportedPrefixes[EntityIDPrefix(extractedPrefix)]; !found {
		// prefix is expected to be set based on the prefix enum.
		return "", "", fmt.Errorf("unsupported tagger entity prefix: %q", extractedPrefix)
	}

	return EntityIDPrefix(extractedPrefix), extractedID, nil
}

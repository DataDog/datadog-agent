// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// EntityID represents a tagger entityID
// An EntityID should be identified by a prefix and an id, and is represented as {prefix}://{id}
type EntityID interface {
	// GetID returns a prefix-specific id (i.e. an ID unique given prefix)
	GetID() string
	// GetPrefix returns the prefix of the EntityID
	GetPrefix() EntityIDPrefix
	// String returns a string representation of EntityID under the format {prefix}://{id}
	String() string
}

// defaultEntityID implements EntityID as a plain string id
type defaultEntityID string

// GetID implements EntityID#GetID
func (de defaultEntityID) GetID() string {
	if strings.Contains(string(de), "://") {
		return string(de)
	}

	parts := strings.Split(string(de), "://")
	if len(parts) != 2 {
		return ""
	}

	return parts[0]
}

// GetPrefix implements EntityID#GetPrefix
func (de defaultEntityID) GetPrefix() EntityIDPrefix {
	if strings.Contains(string(de), "://") {
		return EntityIDPrefix(de)
	}

	parts := strings.Split(string(de), "://")
	if len(parts) != 2 {
		return ""
	}

	return EntityIDPrefix(parts[0])
}

// String implements EntityID#String
func (de defaultEntityID) String() string {
	return string(de)
}

func newDefaultEntityID(id string) EntityID {
	return defaultEntityID(id)
}

// compositeEntityID implements EntityID as a struct of prefix and id
type compositeEntityID struct {
	Prefix EntityIDPrefix
	ID     string
}

// GetPrefix implements EntityID#GetPrefix
func (eid compositeEntityID) GetPrefix() EntityIDPrefix {
	return eid.Prefix
}

// GetID implements EntityID#GetID
func (eid compositeEntityID) GetID() string {
	return eid.ID
}

// String implements EntityID#String
func (eid compositeEntityID) String() string {
	return eid.Prefix.ToUID(eid.ID)
}

// newcompositeEntityID returns a new EntityID based on a prefix and an id
func newCompositeEntityID(prefix EntityIDPrefix, id string) EntityID {
	return compositeEntityID{
		Prefix: prefix,
		ID:     id,
	}
}

// NewEntityID builds and returns an EntityID object based on plain string uid
// Currently, it defaults to the default implementation of EntityID as a plain string
func NewEntityID(prefix EntityIDPrefix, id string) EntityID {
	if config.Datadog().GetBool("tagger.tagstore_use_composite_entity_id") {
		return newCompositeEntityID(prefix, id)
	}
	return newDefaultEntityID(fmt.Sprintf("%s://%s", prefix, id))
}

// NewEntityIDFromString constructs EntityID from a plain string id
func NewEntityIDFromString(plainStringID string) (EntityID, error) {
	if config.Datadog().GetBool("tagger.tagstore_use_composite_entity_id") {
		if !strings.Contains(plainStringID, "://") {
			return nil, fmt.Errorf("unsupported tagger entity id format %q, correct format is `{prefix}://{id}`", plainStringID)
		}
		parts := strings.Split(plainStringID, "://")
		return newCompositeEntityID(EntityIDPrefix(parts[0]), parts[1]), nil
	}
	return newDefaultEntityID(plainStringID), nil
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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"fmt"
	"strings"

	taggerutils "github.com/DataDog/datadog-agent/pkg/util/tagger"
)

const separator = "://"

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
	parts := strings.SplitN(string(de), separator, 2)

	if len(parts) != 2 {
		return ""
	}

	return parts[1]
}

// GetPrefix implements EntityID#GetPrefix
func (de defaultEntityID) GetPrefix() EntityIDPrefix {
	parts := strings.SplitN(string(de), separator, 2)

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
	// TODO: use composite entity id always or use component framework for config component
	if taggerutils.ShouldUseCompositeStore() {
		return newCompositeEntityID(prefix, id)
	}
	return newDefaultEntityID(fmt.Sprintf("%s://%s", prefix, id))
}

// NewEntityIDFromString constructs EntityID from a plain string id
func NewEntityIDFromString(plainStringID string) (EntityID, error) {
	if taggerutils.ShouldUseCompositeStore() {
		if !strings.Contains(plainStringID, separator) {
			return nil, fmt.Errorf("unsupported tagger entity id format %q, correct format is `{prefix}://{id}`", plainStringID)
		}
		parts := strings.Split(plainStringID, separator)
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

// globalEntityID is relocated from tagger/common
var globalEntityID = NewEntityID("internal", "global-entity-id")

// GetGlobalEntityID returns the entity ID that holds global tags
func GetGlobalEntityID() EntityID {
	return globalEntityID
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containers

// ContainerEntityName is the entity name applied to all containers
const ContainerEntityName = "container_id"

// EntitySeparator is used to separate the entity name from its ID
const EntitySeparator = "://"

// ContainerEntityPrefix is the prefix that any entity corresponding to a container must have
// It replaces any prior prefix like <runtime>:// in a pod container status.
const ContainerEntityPrefix = ContainerEntityName + EntitySeparator

// BuildEntityName builds a valid entity name for a given container runtime and cid.
func BuildEntityName(runtime, id string) string {
	panic("not called")
}

// BuildTaggerEntityName builds a valid tagger entity name for a given cid.
func BuildTaggerEntityName(id string) string {
	panic("not called")
}

// SplitEntityName returns the prefix and container cid parts of a valid entity name
func SplitEntityName(name string) (string, string) {
	panic("not called")
}

// ContainerIDForEntity extracts the container ID portion of a container entity name
func ContainerIDForEntity(name string) string {
	panic("not called")
}

// IsEntityName tests whether a given entity name is valid
func IsEntityName(name string) bool {
	panic("not called")
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package containers

import (
	"fmt"
	"strings"
)

// ContainerEntityPrefix is the prefix that any entity corresponding to a container must have
// It replaces any prior prefix like <runtime>:// in a pod container status.
const ContainerEntityPrefix = "container_id://"
const entitySeparator = "://"

// BuildEntityName builds a valid entity name for a given container runtime and cid.
// An empty runtime is fine since we hardcode container_id anyway.
// TODO: We stopped using the runtime as a prefix as of 6.13, but keep it in the function signature in case we need it back
// if this doesn't cause any issue in the few coming versions let's remove it
func BuildEntityName(runtime, id string) string {
	if id == "" {
		return ""
	}
	return fmt.Sprintf("%s%s", ContainerEntityPrefix, id)
}

// SplitEntityName returns the prefix and container cid parts of a valid entity name
func SplitEntityName(name string) (string, string) {
	if !IsEntityName(name) {
		return "", ""
	}
	parts := strings.SplitN(name, entitySeparator, 2)
	return parts[0], parts[1]
}

// ContainerIDForEntity extracts the container ID portion of a container entity name
func ContainerIDForEntity(name string) string {
	_, id := SplitEntityName(name)
	return id
}

// IsEntityName tests whether a given entity name is valid
func IsEntityName(name string) bool {
	return strings.Contains(name, entitySeparator)
}

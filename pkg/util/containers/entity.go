// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package containers

import (
	"fmt"
	"strings"
)

const entitySeparator = "://"

// BuildEntityName builds a valid entity name for a given container runtime and cid
func BuildEntityName(runtime, id string) string {
	if id == "" || runtime == "" {
		return ""
	}
	return fmt.Sprintf("%s%s%s", runtime, entitySeparator, id)
}

// SplitEntityName returns the runtime and container cid parts of a valid entity name
func SplitEntityName(name string) (string, string) {
	if !IsEntityName(name) {
		return "", ""
	}
	parts := strings.SplitN(name, entitySeparator, 2)
	return parts[0], parts[1]
}

// RuntimeForEntity extracts the runtime portion of a container entity name
func RuntimeForEntity(name string) string {
	r, _ := SplitEntityName(name)
	return r
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

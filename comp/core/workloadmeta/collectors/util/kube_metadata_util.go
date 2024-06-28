// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// GenerateKubeMetadataEntityID generates and returns a unique entity id for KubernetesMetadata entity
// for namespaced objects, the id will have the format {resourceType}/{namespace}/{name} (e.g. deployments/default/app )
// for cluster scoped objects, the id will have the format {resourceType}//{name} (e.g. nodes//master-node)
func GenerateKubeMetadataEntityID(resource, namespace, name string) workloadmeta.KubeMetadataEntityID {
	return workloadmeta.KubeMetadataEntityID(fmt.Sprintf("%s/%s/%s", resource, namespace, name))
}

// ParseKubeMetadataEntityID parses a metadata entity ID and returns the resource type, namespace and resource name.
// The parsed id should be in the following format: <resourceType>/<namespace>/<name>
// The namespace field is left empty for cluster-scoped objects.
// Examples:
// - deployments/default/app
// - namespaces//default
// If the parsed id is malformatted, this function will return empty strings and a non nil error
func ParseKubeMetadataEntityID(id workloadmeta.KubeMetadataEntityID) (resource, namespace, name string, err error) {
	parts := strings.Split(string(id), "/")
	if len(parts) != 3 {
		err := fmt.Errorf("malformatted metadata entity id: %s", id)
		return "", "", "", err
	}

	return parts[0], parts[1], parts[2], nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build kubeapiserver

// Package util contains utility functions for image metadata collection
package util

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GenerateKubeMetadataEntityID generates and returns a unique entity id for KubernetesMetadata entity
// for namespaced objects, the id will have the format {resourceType}/{namespace}/{name} (e.g. deployments/default/app )
// for cluster scoped objects, the id will have the format {resourceType}//{name} (e.g. node//master-node)
func GenerateKubeMetadataEntityID(resource, namespace, name string) string {
	return fmt.Sprintf("%s/%s/%s", resource, namespace, name)
}

// ParseKubeMetadataEntityID parses a metadata entity ID and returns the resource type, namespace and resource name.
// The parsed id should be in the following format: <resourceType>/<namespace>/<name>
// The namespace field is left empty for cluster-scoped objects.
// Examples:
// - deployments/default/app
// - namespaces//default
// If the parsed id is malformatted, this function will return empty strings
func ParseKubeMetadataEntityID(id string) (string, string, string) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 {
		log.Errorf("malformatted metadata entity id: %s", id)
		return "", "", ""
	}

	return parts[0], parts[1], parts[2]
}

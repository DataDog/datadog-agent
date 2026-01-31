// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicenaming provides CEL-based service name calculation.
//
// These types define the schema exposed to CEL expressions. They are used
// at runtime to map workloadmeta entities to CEL input variables, NOT for
// compile-time type checking (we use DynType for flexibility).
//
// Example CEL expressions:
//   - container['labels']['app']
//   - container['image']['shortname']
//   - container['envs']['DD_SERVICE']
//   - container['ports'][0]['port']
package servicenaming

import "github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"

// CELInput is the input structure for CEL evaluation.
// It contains the context for service name calculation.
type CELInput struct {
	Container *ContainerCEL `cel:"container"`
}

// ContainerCEL represents a container in the CEL environment.
// Maps from workloadmeta.Container.
type ContainerCEL struct {
	// ID is the container ID
	ID string `cel:"id"`

	// Name is the container name
	Name string `cel:"name"`

	// Image contains image information
	Image ImageCEL `cel:"image"`

	// Labels are container-level labels only (from workloadmeta.Container.Labels).
	// Note: Pod-level annotations are NOT included here; they would require
	// looking up the owning pod. For k8s environments, UST labels are typically
	// propagated to container labels by the orchestrator.
	// Access UST tags via: container['labels']['tags.datadoghq.com/my-app.service']
	Labels map[string]string `cel:"labels"`

	// Envs are environment variables (filtered subset available in workloadmeta).
	// Only env vars matching the filter in pkg/util/containers/env_vars_filter.go
	// are available (includes DD_SERVICE, DD_ENV, DD_VERSION, etc.).
	// Access UST env vars via: container['envs']['DD_SERVICE']
	Envs map[string]string `cel:"envs"`

	// Ports are the exposed container ports with full metadata.
	// Access via: container['ports'][0]['port'], container['ports'][0]['protocol']
	Ports []PortCEL `cel:"ports"`
}

// PortCEL represents a container port in the CEL environment.
// Maps from workloadmeta.ContainerPort.
type PortCEL struct {
	// Name is the port name (may be empty)
	Name string `cel:"name"`

	// Port is the port number
	Port int `cel:"port"`

	// Protocol is the protocol (e.g., "tcp", "udp")
	Protocol string `cel:"protocol"`
}

// ImageCEL represents container image information in the CEL environment.
// Maps from workloadmeta.ContainerImage.
type ImageCEL struct {
	// Name is the full image name (e.g., "docker.io/library/redis:latest")
	Name string `cel:"name"`

	// ShortName is the image short name without registry (e.g., "redis")
	ShortName string `cel:"shortname"`

	// Tag is the image tag (e.g., "latest", "v1.2.3")
	Tag string `cel:"tag"`

	// Registry is the image registry (e.g., "docker.io")
	Registry string `cel:"registry"`
}

// ToEngineInput converts CELInput to engine.CELInput by converting structs to maps.
// This conversion is necessary because CEL works with maps for dynamic field access.
func ToEngineInput(input CELInput) engine.CELInput {
	return engine.CELInput{
		Container: convertContainer(input.Container),
	}
}

// convertContainer converts ContainerCEL to a CEL-compatible map (or nil).
// Note: nil maps are normalized to empty maps at the source (buildCELInput),
// so we don't need to check for nil here.
func convertContainer(c *ContainerCEL) map[string]any {
	if c == nil {
		return nil
	}

	// Convert ports to list of maps for CEL access
	ports := make([]map[string]any, len(c.Ports))
	for i, p := range c.Ports {
		ports[i] = map[string]any{
			"name":     p.Name,
			"port":     p.Port,
			"protocol": p.Protocol,
		}
	}

	return map[string]any{
		"id":   c.ID,
		"name": c.Name,
		"image": map[string]any{
			"name":      c.Image.Name,
			"shortname": c.Image.ShortName,
			"tag":       c.Image.Tag,
			"registry":  c.Image.Registry,
		},
		"labels": c.Labels,
		"envs":   c.Envs,
		"ports":  ports,
	}
}

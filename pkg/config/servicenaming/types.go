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
type CELInput struct {
	Container *ContainerCEL `cel:"container"`
}

// ContainerCEL represents a container in CEL expressions (maps from workloadmeta.Container).
type ContainerCEL struct {
	ID     string            `cel:"id"`
	Name   string            `cel:"name"`
	Image  ImageCEL          `cel:"image"`
	Labels map[string]string `cel:"labels"` // Container labels only; pod annotations require separate lookup
	Envs   map[string]string `cel:"envs"`   // Filtered env vars from pkg/util/containers/env_vars_filter.go
	Ports  []PortCEL         `cel:"ports"`
}

// PortCEL represents a container port (maps from workloadmeta.ContainerPort).
type PortCEL struct {
	Name     string `cel:"name"`
	Port     int    `cel:"port"`
	Protocol string `cel:"protocol"`
}

// ImageCEL represents container image information (maps from workloadmeta.ContainerImage).
type ImageCEL struct {
	Name      string `cel:"name"`
	ShortName string `cel:"shortname"`
	Tag       string `cel:"tag"`
	Registry  string `cel:"registry"`
}

// ToEngineInput converts CELInput to engine.CELInput for CEL evaluation.
func ToEngineInput(input CELInput) engine.CELInput {
	return engine.CELInput{
		Container: convertContainer(input.Container),
	}
}

// convertContainer converts ContainerCEL to a map for CEL evaluation.
func convertContainer(c *ContainerCEL) map[string]any {
	if c == nil {
		return nil
	}

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

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicenaming provides CEL-based service name calculation.
package servicenaming

import "github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"

// CELInput is the input structure for CEL evaluation.
type CELInput struct {
	Container *ContainerCEL
}

// ContainerCEL represents a container in CEL expressions (maps from workloadmeta.Container).
type ContainerCEL struct {
	ID     string
	Name   string
	Image  ImageCEL
	Labels map[string]string
	Envs   map[string]string
	Ports  []PortCEL
}

// PortCEL represents a container port (maps from workloadmeta.ContainerPort).
type PortCEL struct {
	Name     string
	Port     int
	Protocol string
}

// ImageCEL represents container image information (maps from workloadmeta.ContainerImage).
type ImageCEL struct {
	Name      string
	ShortName string
	Tag       string
	Registry  string
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

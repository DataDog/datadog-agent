// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package compliance defines common interfaces and types for Compliance Agent
package compliance

// Rule defines a rule in a compliance config
type Rule struct {
	ID           string        `yaml:"id"`
	Scope        Scope         `yaml:"scope"`
	HostSelector *HostSelector `yaml:"hostSelector,omitempty"`
	Resources    []Resource    `yaml:"resources,omitempty"`
}

const (
	// DockerScope const
	DockerScope string = "docker"
	// KubernetesNodeScope const
	KubernetesNodeScope string = "kubernetesNode"
	// KubernetesClusterScope const
	KubernetesClusterScope string = "kubernetesCluster"
)

// Scope defines when a rule can be run based on observed properties of the environment
type Scope struct {
	Docker            bool `yaml:"docker,omitempty"`
	KubernetesNode    bool `yaml:"kubernetesNode,omitempty"`
	KubernetesCluster bool `yaml:"kubernetesCluster,omitempty"`
}

// HostSelector allows to activate/deactivate dynamically based on host properties
type HostSelector struct {
	KubernetesNodeLabels []KubeNodeSelector `yaml:"kubernetesRole,omitempty"`
	KubernetesNodeRole   string             `yaml:"kubernetesNodeRole,omitempty"`
}

// KubeNodeSelector defines selector for a Kubernetes node
type KubeNodeSelector struct {
	Label string `yaml:"label,omitempty"`
	Value string `yaml:"value,omitempty"`
}

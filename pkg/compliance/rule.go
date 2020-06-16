// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package compliance defines common interfaces and types for Compliance Agent
package compliance

// Rule defines a rule in a compliance config
type Rule struct {
	ID        string     `yaml:"id"`
	Scope     Scope      `yaml:"scope,omitempty"`
	Resources []Resource `yaml:"resources,omitempty"`
}

// Scope defines when a rule can be run based on observed properties of the environment
type Scope struct {
	Docker     bool               `yaml:"docker"`
	Kubernetes []KubeNodeSelector `yaml:"kubernetes,omitempty"`
}

// KubeNodeSelector defines selector for a Kubernetes node
type KubeNodeSelector struct {
	Label      string `yaml:"label,omitempty"`
	Annotation string `yaml:"annotation,omitempty"`
	Value      string `yaml:"value,omitempty"`
}

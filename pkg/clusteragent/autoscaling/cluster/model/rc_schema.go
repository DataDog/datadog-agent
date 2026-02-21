// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// ClusterAutoscalingValuesList represents a list of cluster autoscaling recommendations.
// These recommendations are formatted into Karpenter NodePools to be applied in the customer's environment.
type ClusterAutoscalingValuesList struct {
	// Values is a list of ClusterAutoscalingValues
	Values []ClusterAutoscalingValues `json:"values"`
}

// ClusterAutoscalingValues represents the fields in a cluster autoscaling recommendation.
type ClusterAutoscalingValues struct {
	// TargetName refers to the associated NodePool (if exists) that the recommendation was created from
	TargetName string `json:"target_name"`

	// TargetHash is a hash of the original NodePool (if exists) at the time the recommendation was generated
	TargetHash string `json:"target_hash"`

	// Type is the type of Manifest to use
	Type Type `json:"type" jsonschema:"title=Type,description=The type of manifest to use"`

	// Manifest is the manifest of the NodePool
	Manifest Manifest `json:"manifest"`

	// Deprecated fields, these will be removed in a future release

	// Name is the name of the associated Karpenter NodePool object
	Name string `json:"name,omitempty"`

	// RecommendedInstanceTypes are the list of instance types that the NodePool should be restricted to
	RecommendedInstanceTypes []string `json:"recommended_instance_types,omitempty"`

	// Labels are the domain labels that should be present on the NodePool
	Labels []DomainLabel `json:"labels,omitempty"`

	// Taints are the domain taints that should be present on the NodePool
	Taints []Taint `json:"taints,omitempty"`
}

// Type represents the manifest type to use
type Type string

const (
	TypeKarpenterV1 Type = "karpenter_v1"
)

// Manifest contains the scheduling domain specifications for different versions
type Manifest struct {
	KarpenterV1 *karpenterv1.NodePool `json:"karpenter_v1"`
}

// DomainLabel represents the set of domain labels that should be present on the NodePool.
type DomainLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Taint represents the minimum set of taints that should be set on the NodePool
type Taint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

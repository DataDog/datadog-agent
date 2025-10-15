// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package v1 contains vendored types from sigs.k8s.io/gateway-api/apis/v1
// These types are vendored to avoid adding external dependencies
package v1

// Group refers to a Kubernetes Group. It must either be an empty string or a RFC 1123 subdomain.
type Group string

// ObjectName refers to the name of a Kubernetes object
type ObjectName string

// Namespace refers to a Kubernetes Namespace
type Namespace string

// PortNumber defines a network port
type PortNumber int32

// BackendObjectReference identifies an API object within a known namespace
type BackendObjectReference struct {
	// Group is the group of the referent
	Group *Group `json:"group,omitempty"`
	// Kind is kind of the referent
	Kind *string `json:"kind,omitempty"`
	// Name is the name of the referent
	Name ObjectName `json:"name"`
	// Namespace is the namespace of the referent
	Namespace *Namespace `json:"namespace,omitempty"`
	// Port specifies the destination port number to use for this resource
	Port *PortNumber `json:"port,omitempty"`
}

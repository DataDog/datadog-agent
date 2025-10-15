// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package v1beta1 contains vendored types from sigs.k8s.io/gateway-api/apis/v1beta1
// These types are vendored to avoid adding external dependencies
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Group refers to a Kubernetes Group
type Group string

// Kind refers to a Kubernetes Kind
type Kind string

// Namespace refers to a Kubernetes Namespace
type Namespace string

// ObjectName refers to the name of a Kubernetes object
type ObjectName string

// ReferenceGrant identifies kinds of resources in other namespaces that are
// trusted to reference the specified kinds of resources in the same namespace
type ReferenceGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of ReferenceGrant
	Spec ReferenceGrantSpec `json:"spec,omitempty"`
}

// ReferenceGrantSpec identifies a cross namespace relationship that is trusted for Gateway API
type ReferenceGrantSpec struct {
	// From describes the trusted namespaces and kinds that can reference the
	// resources described in "To"
	From []ReferenceGrantFrom `json:"from"`

	// To describes the resources that may be referenced by the resources
	// described in "From"
	To []ReferenceGrantTo `json:"to,omitempty"`
}

// ReferenceGrantFrom describes trusted namespaces and kinds
type ReferenceGrantFrom struct {
	// Group is the group of the referent
	Group Group `json:"group"`

	// Kind is the kind of the referent
	Kind Kind `json:"kind"`

	// Namespace is the namespace of the referent
	Namespace Namespace `json:"namespace"`
}

// ReferenceGrantTo describes what Kinds are allowed as targets of the references
type ReferenceGrantTo struct {
	// Group is the group of the referent
	Group Group `json:"group,omitempty"`

	// Kind is the kind of the referent
	Kind Kind `json:"kind"`

	// Name is the name of the referent. When unspecified, this policy
	// refers to all resources of the specified Group and Kind in the local
	// namespace.
	Name *ObjectName `json:"name,omitempty"`
}

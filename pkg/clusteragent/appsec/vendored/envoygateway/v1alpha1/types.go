// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package v1alpha1 contains vendored types from github.com/envoyproxy/gateway/api/v1alpha1
// These types are vendored to avoid adding external dependencies
package v1alpha1

import (
	gwapiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/vendored/gatewayapi/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvoyExtensionPolicy allows the user to configure various envoy extensibility options for the Gateway
type EnvoyExtensionPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of EnvoyExtensionPolicy
	Spec EnvoyExtensionPolicySpec `json:"spec"`
}

// EnvoyExtensionPolicySpec defines the desired state of EnvoyExtensionPolicy
type EnvoyExtensionPolicySpec struct {
	// PolicyTargetReferences identifies the target resources to which the policy will be applied
	PolicyTargetReferences `json:",inline"`

	// ExtProc defines the configuration for External Processing filter
	ExtProc []ExtProc `json:"extProc,omitempty"`
}

// PolicyTargetReferences identifies the target resources to which the policy will be applied
type PolicyTargetReferences struct {
	// TargetRef is the name of the resource this policy is being attached to
	// Deprecated: Use TargetRefs instead
	TargetRef *TargetRef `json:"targetRef,omitempty"`

	// TargetRefs are the names of the resources this policy is being attached to
	TargetRefs []TargetRef `json:"targetRefs,omitempty"`

	// TargetSelectors is used to select resources based on labels
	TargetSelectors []TargetSelector `json:"targetSelectors,omitempty"`
}

// TargetRef identifies an API object to apply policy to
type TargetRef struct {
	// Group is the group of the target resource
	Group *gwapiv1.Group `json:"group"`

	// Kind is kind of the target resource
	Kind string `json:"kind"`

	// Name is the name of the target resource
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the referent
	Namespace *gwapiv1.Namespace `json:"namespace,omitempty"`

	// SectionName is the name of a section within the target resource
	SectionName *string `json:"sectionName,omitempty"`
}

// TargetSelector defines a selector for resources
type TargetSelector struct {
	// Group is the group that this selector targets
	Group *gwapiv1.Group `json:"group,omitempty"`

	// Kind is the kind that this selector targets
	Kind string `json:"kind"`

	// MatchLabels are the labels to match
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// ExtProc defines the configuration for the External Processing filter
type ExtProc struct {
	// BackendRefs defines the referenced backend services for external processing
	// Deprecated: Use BackendCluster instead
	BackendRefs []BackendRef `json:"backendRefs,omitempty"`

	// BackendCluster defines the referenced backend cluster for external processing
	BackendCluster BackendCluster `json:"backendCluster,omitempty"`

	// FailOpen defines whether to allow traffic if the external processor is unavailable
	FailOpen *bool `json:"failOpen,omitempty"`

	// ProcessingMode describes which parts of the request and response to process
	ProcessingMode *ExtProcProcessingMode `json:"processingMode,omitempty"`
}

// BackendCluster defines a backend cluster
type BackendCluster struct {
	// BackendRefs references the backend services
	BackendRefs []BackendRef `json:"backendRefs,omitempty"`
}

// BackendRef identifies a backend
type BackendRef struct {
	// BackendObjectReference references a Kubernetes object
	gwapiv1.BackendObjectReference `json:",inline"`

	// Weight specifies the proportion of requests forwarded to the referenced backend
	Weight *int32 `json:"weight,omitempty"`
}

// ExtProcProcessingMode defines the processing mode for external processing
type ExtProcProcessingMode struct {
	// Request defines the processing mode for requests
	Request *ProcessingModeOptions `json:"request,omitempty"`

	// Response defines the processing mode for responses
	Response *ProcessingModeOptions `json:"response,omitempty"`

	// AllowModeOverride allows the external processor to override the processing mode
	AllowModeOverride bool `json:"allowModeOverride,omitempty"`
}

// ProcessingModeOptions defines what parts of the request/response should be sent
type ProcessingModeOptions struct {
	// Body defines whether to send the body
	Body *string `json:"body,omitempty"`

	// Header defines whether to send headers
	Header *string `json:"header,omitempty"`
}

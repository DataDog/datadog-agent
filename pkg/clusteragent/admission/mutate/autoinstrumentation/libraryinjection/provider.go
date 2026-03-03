// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package libraryinjection provides different strategies for injecting APM libraries into pods.
// It defines a common interface (LibraryInjectionProvider) that can be implemented by different
// injection mechanisms such as init containers and CSI volumes.
package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/version"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// MutationStatus represents the outcome of a mutation operation.
type MutationStatus string

const (
	// MutationStatusInjected indicates the injection was successful.
	MutationStatusInjected MutationStatus = "injected"
	// MutationStatusSkipped indicates the injection was skipped (e.g., insufficient resources).
	MutationStatusSkipped MutationStatus = "skipped"
	// MutationStatusError indicates an error occurred during injection.
	MutationStatusError MutationStatus = "error"
)

// MutationContext contains data to pass between injection calls.
// This allows InjectInjector to compute values that InjectLibrary can reuse.
type MutationContext struct {
	// ResourceRequirements are the computed resource requirements for init containers.
	ResourceRequirements corev1.ResourceRequirements
	// InitSecurityContext is the security context to apply to init containers.
	// This is computed once based on namespace labels and reused for all injections.
	InitSecurityContext *corev1.SecurityContext
}

// MutationResult contains the result of a mutation operation.
type MutationResult struct {
	// Status indicates the outcome of the mutation.
	Status MutationStatus
	// Err contains the error that occurred during mutation (for MutationStatusError or MutationStatusSkipped).
	Err error
	// Context contains data to pass to subsequent injection calls.
	Context MutationContext
}

// InjectorConfig contains the configuration needed to inject the APM injector component.
type InjectorConfig struct {
	// Package contains the OCI package reference.
	Package LibraryImage
}

// LibraryConfig contains the configuration needed to inject a language-specific tracing library.
type LibraryConfig struct {
	// Language is the programming language (e.g., "java", "python", "js").
	Language string
	// Package contains the OCI package reference.
	Package LibraryImage
	// ContainerName is the target container name (empty means all containers).
	ContainerName string
	// Context contains data from a previous InjectInjector call.
	// This is used to pass computed values like resource requirements and security context.
	Context MutationContext
}

// LibraryInjectionConfig contains all configuration needed to perform APM library injection.
type LibraryInjectionConfig struct {
	// InjectionMode determines the method for injecting libraries into pods.
	// Possible values: "auto" (default), "init_container" and "csi".
	InjectionMode string

	// DefaultResourceRequirements are the default resource requirements for init containers.
	// If empty, the provider will compute requirements based on the pod's existing resources.
	DefaultResourceRequirements map[corev1.ResourceName]resource.Quantity

	// InitSecurityContext is the security context to apply to init containers.
	// If nil, the provider will compute one based on namespace labels.
	InitSecurityContext *corev1.SecurityContext

	// ContainerFilter is an optional filter to select which containers to mutate.
	// If nil, all containers are mutated.
	ContainerFilter func(*corev1.Container) bool

	// Wmeta is the workloadmeta component for accessing namespace metadata.
	// Used to resolve security context based on namespace labels.
	Wmeta workloadmeta.Component

	// KubeServerVersion is the Kubernetes API server version.
	// Used for gating features that depend on cluster version support (e.g. image volumes).
	KubeServerVersion *version.Info

	// Debug enables debug mode for the APM libraries.
	// When true, additional debug environment variables are injected.
	Debug bool

	// Injector contains configuration for the APM injector.
	Injector InjectorConfig

	// Libraries is the list of language-specific libraries to inject.
	Libraries []LibraryConfig

	// AutoDetected indicates if the injection was triggered by auto-detection (for metrics).
	AutoDetected bool

	// InjectionType identifies the type of injection, e.g. "single step" or "lib injection" (for metrics).
	InjectionType string
}

// LibraryInjectionProvider defines the strategy for injecting APM libraries into pods.
// Providers mutate the pod directly and return a status indicating the result.
//
// Different implementations can use different mechanisms:
// - InitContainerProvider: Uses init containers with EmptyDir volumes
// - CSIProvider: Uses a CSI driver to mount library files
type LibraryInjectionProvider interface {
	// InjectInjector mutates the pod to add the APM injector component.
	// The injector is responsible for the LD_PRELOAD mechanism that enables auto-instrumentation.
	// It adds volumes, volume mounts, and optionally init containers to the pod.
	InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult

	// InjectLibrary mutates the pod to add a language-specific tracing library.
	// It adds volumes, volume mounts, and optionally init containers for the library.
	// Returns MutationStatusError if the language is not supported.
	InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult
}

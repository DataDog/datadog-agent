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

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// mutationStatus represents the outcome of a mutation operation.
type mutationStatus string

const (
	// mutationStatusInjected indicates the injection was successful.
	mutationStatusInjected mutationStatus = "injected"
	// mutationStatusSkipped indicates the injection was skipped (e.g., insufficient resources).
	mutationStatusSkipped mutationStatus = "skipped"
	// mutationStatusError indicates an error occurred during injection.
	mutationStatusError mutationStatus = "error"
)

// mutationContext contains data to pass between injection calls.
// This allows injectInjector to compute values that injectLibrary can reuse.
type mutationContext struct {
	// resourceRequirements are the computed resource requirements for init containers.
	resourceRequirements corev1.ResourceRequirements
	// initSecurityContext is the security context to apply to init containers.
	// This is computed once based on namespace labels and reused for all injections.
	initSecurityContext *corev1.SecurityContext
}

// mutationResult contains the result of a mutation operation.
type mutationResult struct {
	// status indicates the outcome of the mutation.
	status mutationStatus
	// err contains the error that occurred during mutation (for mutationStatusError or mutationStatusSkipped).
	err error
	// context contains data to pass to subsequent injection calls.
	context mutationContext
}

// ResolvedImage contains an image reference and its canonical version.
// The canonical version is the human-readable version (e.g., "1.2.3") as opposed
// to a digest (e.g., "sha256:abc123"). It's used for annotations/telemetry.
type ResolvedImage struct {
	// Image is the full image reference (e.g., "gcr.io/datadoghq/dd-lib-java-init@sha256:abc123").
	Image string
	// CanonicalVersion is the human-readable version (e.g., "1.2.3"), empty if not resolved.
	CanonicalVersion string
}

// InjectorConfig contains the configuration needed to inject the APM injector component.
type InjectorConfig struct {
	ResolvedImage
}

// LibraryConfig contains the configuration needed to inject a language-specific tracing library.
type LibraryConfig struct {
	// Language is the programming language (e.g., "java", "python", "js").
	Language string
	// ResolvedImage contains the image reference and canonical version.
	ResolvedImage
	// ContainerName is the target container name (empty means all containers).
	ContainerName string
	// Context contains data from a previous injectInjector call.
	// This is used to pass computed values like resource requirements and security context.
	Context mutationContext
}

// LibraryInjectionConfig contains all configuration needed to perform APM library injection.
type LibraryInjectionConfig struct {
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

// libraryInjectionProvider defines the strategy for injecting APM libraries into pods.
// Providers mutate the pod directly and return a status indicating the result.
//
// Different implementations can use different mechanisms:
// - initContainerProvider: Uses init containers with EmptyDir volumes
// - (future) CSI provider: Uses a CSI driver to mount library files
type libraryInjectionProvider interface {
	// injectInjector mutates the pod to add the APM injector component.
	// The injector is responsible for the LD_PRELOAD mechanism that enables auto-instrumentation.
	// It adds volumes, volume mounts, and optionally init containers to the pod.
	injectInjector(pod *corev1.Pod, cfg InjectorConfig) mutationResult

	// injectLibrary mutates the pod to add a language-specific tracing library.
	// It adds volumes, volume mounts, and optionally init containers for the library.
	// Returns mutationStatusError if the language is not supported.
	injectLibrary(pod *corev1.Pod, cfg LibraryConfig) mutationResult
}

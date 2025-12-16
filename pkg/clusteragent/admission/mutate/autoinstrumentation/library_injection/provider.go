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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// InjectionMode represents the method used to inject APM libraries into pods.
type InjectionMode string

const (
	// InjectionModeInitContainer uses init containers to copy library files into an EmptyDir volume.
	// This is the traditional/default injection method.
	InjectionModeInitContainer InjectionMode = "init_container"

	// InjectionModeCSI uses a CSI driver to mount library files directly into the pod.
	InjectionModeCSI InjectionMode = "csi"
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

// MutationResult contains the result of a mutation operation.
type MutationResult struct {
	// Status indicates the outcome of the mutation.
	Status MutationStatus
	// Message contains additional information (skip reason or error details).
	Message string
}

// InjectorConfig contains the configuration needed to inject the APM injector component.
type InjectorConfig struct {
	// Image is the full image reference for the APM injector.
	Image string
	// Registry is the container registry to use.
	Registry string
	// InitSecurityContext is the security context to apply to init containers for this specific injection.
	// This may differ from ProviderConfig.InitSecurityContext if the namespace requires a specific context.
	InitSecurityContext *corev1.SecurityContext
}

// InjectorEnvConfig contains the configuration for injector environment variables.
// This is separate from InjectorConfig because env vars are added by the mutator core,
// not by the individual providers.
type InjectorEnvConfig struct {
	// Debug enables debug mode for the injector.
	Debug bool
	// InjectTime is the time when the injection was initiated.
	InjectTime time.Time
}

// LibraryConfig contains the configuration needed to inject a language-specific tracing library.
type LibraryConfig struct {
	// Language is the programming language (e.g., "java", "python", "js").
	Language string
	// Image is the full image reference for the library init container.
	Image string
	// Registry is the container registry.
	Registry string
	// Repository is the image repository name (e.g., "dd-lib-java-init").
	Repository string
	// Tag is the image tag/version.
	Tag string
	// ContainerName is the target container name (empty means all containers).
	ContainerName string
	// InitSecurityContext is the security context to apply to init containers for this specific injection.
	// This may differ from ProviderConfig.InitSecurityContext if the namespace requires a specific context.
	InitSecurityContext *corev1.SecurityContext
}

// ProviderConfig contains the configuration passed to providers.
// This allows providers to access settings they need for injection.
type ProviderConfig struct {
	// DefaultResourceRequirements are the default resource requirements for init containers.
	// If empty, the provider will compute requirements based on the pod's existing resources.
	DefaultResourceRequirements map[corev1.ResourceName]resource.Quantity

	// InitSecurityContext is the security context to apply to init containers.
	// If nil, the provider may compute one based on namespace labels.
	InitSecurityContext *corev1.SecurityContext

	// ContainerFilter is an optional filter to select which containers to mutate.
	// If nil, all containers are mutated.
	ContainerFilter func(*corev1.Container) bool
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

// NewProvider creates a LibraryInjectionProvider based on the specified injection mode.
// The providerCfg is used to configure the provider with necessary settings.
func NewProvider(mode InjectionMode, providerCfg ProviderConfig) LibraryInjectionProvider {
	switch mode {
	case InjectionModeCSI:
		return NewCSIProvider(providerCfg)
	case InjectionModeInitContainer:
		fallthrough
	default:
		return NewInitContainerProvider(providerCfg)
	}
}

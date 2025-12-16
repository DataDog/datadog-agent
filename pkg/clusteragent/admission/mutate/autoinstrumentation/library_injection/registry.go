// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// InjectionModeAnnotation is the annotation key to override the injection mode per-pod.
	// Valid values: "init_container" and "csi"
	InjectionModeAnnotation = "admission.datadoghq.com/apm-inject.injection-mode"
)

// Registry holds all available injection providers and handles provider selection.
type Registry struct {
	// defaultMode is the default injection mode to use when no annotation is present.
	defaultMode InjectionMode
	// providers maps injection modes to their corresponding providers.
	providers map[InjectionMode]LibraryInjectionProvider
}

// NewRegistry creates a new Registry with all available providers.
func NewRegistry(defaultMode InjectionMode, cfg ProviderConfig) *Registry {
	// Treat empty string as init_container for backwards compatibility
	if defaultMode == "" {
		defaultMode = InjectionModeInitContainer
	}

	initContainerProvider := NewInitContainerProvider(cfg)

	return &Registry{
		defaultMode: defaultMode,
		providers: map[InjectionMode]LibraryInjectionProvider{
			InjectionModeInitContainer: initContainerProvider,
			InjectionModeCSI:           NewCSIProvider(cfg),
			// Empty string is treated as init_container for backwards compatibility
			"": initContainerProvider,
		},
	}
}

// DefaultProvider returns the default injection provider based on the configured mode.
func (r *Registry) DefaultProvider() LibraryInjectionProvider {
	return r.providers[r.defaultMode]
}

// DefaultMode returns the default injection mode.
func (r *Registry) DefaultMode() InjectionMode {
	return r.defaultMode
}

// GetProvider returns the provider for the specified injection mode.
// If the mode is not found, it returns the default provider.
func (r *Registry) GetProvider(mode InjectionMode) LibraryInjectionProvider {
	if provider, exists := r.providers[mode]; exists {
		return provider
	}
	return r.DefaultProvider()
}

// GetProviderForPod returns the appropriate injection provider for a pod.
// It checks for the injection mode annotation on the pod and returns the corresponding provider.
// If no annotation is present or the value is invalid, it returns the default provider.
func (r *Registry) GetProviderForPod(pod *corev1.Pod) LibraryInjectionProvider {
	if pod.Annotations == nil {
		return r.DefaultProvider()
	}

	modeStr, ok := pod.Annotations[InjectionModeAnnotation]
	if !ok || modeStr == "" {
		return r.DefaultProvider()
	}

	mode := InjectionMode(modeStr)
	if provider, exists := r.providers[mode]; exists {
		log.Debugf("Using injection mode %q from pod annotation for pod %s/%s", modeStr, pod.Namespace, pod.Name)
		return provider
	}

	log.Warnf("Unknown injection mode %q in pod annotation for pod %s/%s, using default", modeStr, pod.Namespace, pod.Name)
	return r.DefaultProvider()
}

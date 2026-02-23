// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
)

// AutoProvider implements LibraryInjectionProvider.
// It will be capable of determining the best provider based on the pod and runtime environment.
type AutoProvider struct {
	realProvider LibraryInjectionProvider
}

// NewAutoProvider creates a new AutoProvider.
func NewAutoProvider(cfg LibraryInjectionConfig) *AutoProvider {
	return &AutoProvider{
		realProvider: NewInitContainerProvider(cfg),
	}
}

// InjectInjector mutates the pod to add the APM injector.
func (p *AutoProvider) InjectInjector(pod *corev1.Pod, cfg InjectorConfig) MutationResult {
	return p.realProvider.InjectInjector(pod, cfg)
}

// InjectLibrary mutates the pod to add a language-specific tracing library.
func (p *AutoProvider) InjectLibrary(pod *corev1.Pod, cfg LibraryConfig) MutationResult {
	return p.realProvider.InjectLibrary(pod, cfg)
}

// Verify that AutoProvider implements LibraryInjectionProvider.
var _ LibraryInjectionProvider = (*AutoProvider)(nil)

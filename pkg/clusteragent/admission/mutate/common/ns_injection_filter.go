// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"
)

// InjectionFilter encapsulates the logic for deciding whether
// we can do pod mutation based on a NSFilter (NamespaceInjectionFilter).
// See: InjectionFilter.ShouldMutatePod.
type InjectionFilter struct {
	NSFilter NamespaceInjectionFilter
}

// ShouldMutatePod checks if a pod is mutable per explicit rules and
// the NSFilter if InjectionFilter has one.
func (f InjectionFilter) ShouldMutatePod(pod *corev1.Pod) bool {
	if val, ok := IsExplicitPodMutationEnabled(pod); ok {
		return val
	}

	if f.NSFilter != nil && f.NSFilter.IsNamespaceEligible(pod.Namespace) {
		return true
	}

	return ShouldMutateUnlabelledPods()
}

// NamespaceInjectionFilter represents a contract to be able to filter out which pods are
// eligible for mutation/injection.
//
// This exists to separate the direct implementation in the autoinstrumentation package and
// its dependencies in other webhooks.
//
// See autoinstrumentation.GetInjectionFilter.
type NamespaceInjectionFilter interface {
	// IsNamespaceEligible returns true if a namespace is eligible for injection/mutation.
	IsNamespaceEligible(ns string) bool
	// Err returns an error if creation of the NamespaceInjectionFilter failed.
	Err() error
}

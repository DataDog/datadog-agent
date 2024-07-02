// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	corev1 "k8s.io/api/core/v1"
)

// InjectionFilter represents a contract to be able to filter out which pods are
// eligible for mutation/injection.
//
// See [autoinstrumentation.GetInjectionFilter].
type InjectionFilter interface {
	ShouldInjectPod(pod *corev1.Pod) bool
	IsNamespaceEligible(ns string) bool
}

// MockInjectionFilter creates an InjectionFilter for testing.
func MockInjectionFilter(enabledNamespaces []string) InjectionFilter {
	set := map[string]struct{}{}
	for _, ns := range enabledNamespaces {
		set[ns] = struct{}{}
	}
	return &mockInjectionFilter{namespaces: set}
}

type mockInjectionFilter struct {
	namespaces map[string]struct{}
}

func (f *mockInjectionFilter) ShouldInjectPod(pod *corev1.Pod) bool {
	shouldMutate, _ := ShouldMutatePod(
		pod,
		func() bool { return f.IsNamespaceEligible(pod.Namespace) },
		ShouldMutateUnlabelledPods,
	)
	return shouldMutate
}

func (f *mockInjectionFilter) IsNamespaceEligible(ns string) bool {
	if f.namespaces == nil {
		return false
	}
	_, exists := f.namespaces[ns]
	return exists
}

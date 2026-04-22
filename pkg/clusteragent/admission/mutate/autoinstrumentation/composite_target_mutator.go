// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
)

type compositeTargetMutator struct {
	local *TargetMutator
	rc    *rcTargetProvider
}

func newCompositeTargetMutator(local *TargetMutator, rc *rcTargetProvider) *compositeTargetMutator {
	return &compositeTargetMutator{
		local: local,
		rc:    rc,
	}
}

func (m *compositeTargetMutator) MutatePod(pod *corev1.Pod, ns string, dc dynamic.Interface) (bool, error) {
	if m.rc != nil {
		if rcMutator := m.rc.Current(); rcMutator != nil {
			mutated, err := rcMutator.MutatePod(pod, ns, dc)
			if mutated || err != nil {
				return mutated, err
			}
		}
	}

	return m.local.MutatePod(pod, ns, dc)
}

func (m *compositeTargetMutator) ShouldMutatePod(pod *corev1.Pod) bool {
	if m.rc != nil {
		if rcMutator := m.rc.Current(); rcMutator != nil && rcMutator.ShouldMutatePod(pod) {
			return true
		}
	}

	return m.local.ShouldMutatePod(pod)
}

func (m *compositeTargetMutator) IsNamespaceEligible(namespace string) bool {
	if m.rc != nil {
		if rcMutator := m.rc.Current(); rcMutator != nil && rcMutator.IsNamespaceEligible(namespace) {
			return true
		}
	}

	return m.local.IsNamespaceEligible(namespace)
}

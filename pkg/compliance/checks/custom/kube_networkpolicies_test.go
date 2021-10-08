// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package custom

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func newNamespace(ns string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
}

func newNetworkPolicy(ns, name string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkPolicy",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
}

func TestKubeNetworkPolicies(t *testing.T) {
	tests := []kubeApiserverFixture{
		{
			name:      "No network policies",
			checkFunc: kubernetesNetworkPoliciesCheck,
			objects: []runtime.Object{
				newNamespace("ns1"),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldName:    "ns1",
					compliance.KubeResourceFieldKind:    "Namespace",
					compliance.KubeResourceFieldVersion: "v1",
					compliance.KubeResourceFieldGroup:   "",
				},
				Aggregated: true,
			},
		},
		{
			name:      "Matching policies",
			checkFunc: kubernetesNetworkPoliciesCheck,
			objects: []runtime.Object{
				newNamespace("ns1"),
				newNetworkPolicy("ns1", "policy1"),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:    "ns1",
					compliance.KubeResourceFieldKind:    "Namespace",
					compliance.KubeResourceFieldVersion: "v1",
					compliance.KubeResourceFieldGroup:   "",
				},
				Aggregated: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

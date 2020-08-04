// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package custom

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestKubeNetworkPolicies(t *testing.T) {
	tests := []kubeApiserverFixture{
		{
			name:      "No network policies",
			checkFunc: kubernetesNetworkPoliciesCheck,
			objects: []runtime.Object{
				newUnstructured("v1", "Namespace", "", "ns1", nil),
			},
			expectReport: &compliance.Report{
				Passed: false,
				Data: event.Data{
					compliance.KubeResourceFieldName:    "ns1",
					compliance.KubeResourceFieldKind:    "Namespace",
					compliance.KubeResourceFieldVersion: "v1",
					compliance.KubeResourceFieldGroup:   "",
				},
			},
		},
		{
			name:      "Matching policies",
			checkFunc: kubernetesNetworkPoliciesCheck,
			objects: []runtime.Object{
				newUnstructured("v1", "Namespace", "", "ns1", nil),
				newUnstructured("networking.k8s.io/v1", "NetworkPolicy", "ns1", "policy1", nil),
			},
			expectReport: &compliance.Report{
				Passed: true,
				Data: event.Data{
					compliance.KubeResourceFieldName:    "ns1",
					compliance.KubeResourceFieldKind:    "Namespace",
					compliance.KubeResourceFieldVersion: "v1",
					compliance.KubeResourceFieldGroup:   "",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t)
		})
	}
}

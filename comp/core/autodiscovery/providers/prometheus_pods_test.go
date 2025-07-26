// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

func TestGetConfigErrors(t *testing.T) {
	tests := []struct {
		name     string
		pods     []*kubelet.Pod
		wantErrs bool
	}{
		{
			name: "pod with invalid port annotation",
			pods: []*kubelet.Pod{
				{
					Metadata: kubelet.PodMetadata{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-uid",
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
							"prometheus.io/port":   "invalid",
						},
					},
					Spec: kubelet.Spec{
						Containers: []kubelet.ContainerSpec{
							{
								Name: "test-container",
								Ports: []kubelet.ContainerPortSpec{
									{ContainerPort: 8080},
								},
							},
						},
					},
					Status: kubelet.Status{
						AllContainers: []kubelet.ContainerStatus{
							{Name: "test-container", ID: "test-id"},
						},
					},
				},
			},
			wantErrs: true,
		},
		{
			name: "pod with port annotation but no matching container",
			pods: []*kubelet.Pod{
				{
					Metadata: kubelet.PodMetadata{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-uid",
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
							"prometheus.io/port":   "9999",
						},
					},
					Spec: kubelet.Spec{
						Containers: []kubelet.ContainerSpec{
							{
								Name: "test-container",
								Ports: []kubelet.ContainerPortSpec{
									{ContainerPort: 8080}, // Different port
								},
							},
						},
					},
					Status: kubelet.Status{
						AllContainers: []kubelet.ContainerStatus{
							{Name: "test-container", ID: "test-id"},
						},
					},
				},
			},
			wantErrs: true,
		},
		{
			name: "valid pod should not generate errors",
			pods: []*kubelet.Pod{
				{
					Metadata: kubelet.PodMetadata{
						Name:      "test-pod",
						Namespace: "default",
						UID:       "test-uid",
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
						},
					},
					Spec: kubelet.Spec{
						Containers: []kubelet.ContainerSpec{
							{
								Name: "test-container",
								Ports: []kubelet.ContainerPortSpec{
									{ContainerPort: 8080},
								},
							},
						},
					},
					Status: kubelet.Status{
						AllContainers: []kubelet.ContainerStatus{
							{Name: "test-container", ID: "test-id"},
						},
					},
				},
			},
			wantErrs: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := NewPrometheusPodsConfigProvider(nil, nil)
			require.NoError(t, err)

			prometheusProvider := provider.(*PrometheusPodsConfigProvider)

			_ = prometheusProvider.parsePodlist(test.pods)
			errors := provider.GetConfigErrors()

			if test.wantErrs {
				assert.NotEmpty(t, errors)
			} else {
				assert.Empty(t, errors)
			}
		})
	}
}

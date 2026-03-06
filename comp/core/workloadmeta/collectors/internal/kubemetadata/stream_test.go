// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestApplyResponse(t *testing.T) {
	tests := []struct {
		name                string
		initialPodServices  map[string][]string
		initialNamespaces   map[string]namespaceMetadata
		initialActive       bool
		response            *pb.KubeMetadataStreamResponse
		expectedPodServices map[string][]string
		expectedNamespaces  map[string]namespaceMetadata
	}{
		{
			name:               "full state with empty initial state",
			initialPodServices: nil,
			initialNamespaces:  nil,
			initialActive:      false,
			response: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "test-namespace-1",
						PodName:      "pod1",
						ServiceNames: []string{"svc1", "svc2"},
						Type:         pb.KubeMetadataEventType_SET,
					},
					{
						Namespace:    "test-namespace-2",
						PodName:      "pod2",
						ServiceNames: []string{"svc3"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace:   "test-namespace-1",
						Labels:      map[string]string{"l1": "v1"},
						Annotations: map[string]string{"a1": "v2"},
						Type:        pb.KubeMetadataEventType_SET,
					},
				},
			},
			expectedPodServices: map[string][]string{
				"test-namespace-1/pod1": {"svc1", "svc2"},
				"test-namespace-2/pod2": {"svc3"},
			},
			expectedNamespaces: map[string]namespaceMetadata{
				"test-namespace-1": {
					labels:      map[string]string{"l1": "v1"},
					annotations: map[string]string{"a1": "v2"},
				},
			},
		},
		{
			name: "full state response replaces existing data",
			initialPodServices: map[string][]string{
				"test-namespace/old-pod": {"old-svc"},
			},
			initialNamespaces: map[string]namespaceMetadata{
				"old-ns": {labels: map[string]string{"l1": "v1"}},
			},
			initialActive: true,
			response: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "test-namespace",
						PodName:      "new-pod",
						ServiceNames: []string{"new-svc"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace: "new-ns",
						Labels:    map[string]string{"l1": "v1"},
						Type:      pb.KubeMetadataEventType_SET,
					},
				},
			},
			expectedPodServices: map[string][]string{
				"test-namespace/new-pod": {"new-svc"},
			},
			expectedNamespaces: map[string]namespaceMetadata{
				"new-ns": {
					labels: map[string]string{"l1": "v1"},
				},
			},
		},
		{
			name: "incremental set",
			initialPodServices: map[string][]string{
				"test-namespace/pod1": {"svc1"},
			},
			initialNamespaces: map[string]namespaceMetadata{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
			initialActive: true,
			response: &pb.KubeMetadataStreamResponse{
				IsFullState: false,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "test-namespace",
						PodName:      "pod1",
						ServiceNames: []string{"svc1", "svc2"},
						Type:         pb.KubeMetadataEventType_SET,
					},
					{
						Namespace:    "test-namespace",
						PodName:      "pod2",
						ServiceNames: []string{"svc3"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace: "ns2",
						Labels:    map[string]string{"l1": "v2"},
						Type:      pb.KubeMetadataEventType_SET,
					},
				},
			},
			expectedPodServices: map[string][]string{
				"test-namespace/pod1": {"svc1", "svc2"},
				"test-namespace/pod2": {"svc3"},
			},
			expectedNamespaces: map[string]namespaceMetadata{
				"ns1": {labels: map[string]string{"l1": "v1"}},
				"ns2": {
					labels:      map[string]string{"l1": "v2"},
					annotations: nil,
				},
			},
		},
		{
			name: "unset",
			initialPodServices: map[string][]string{
				"test-namespace/pod1": {"svc1"},
				"test-namespace/pod2": {"svc2"},
			},
			initialNamespaces: map[string]namespaceMetadata{
				"ns1": {labels: map[string]string{"l1": "v1"}},
				"ns2": {labels: map[string]string{"l1": "v2"}},
			},
			initialActive: true,
			response: &pb.KubeMetadataStreamResponse{
				IsFullState: false,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace: "test-namespace",
						PodName:   "pod1",
						Type:      pb.KubeMetadataEventType_UNSET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace: "ns1",
						Type:      pb.KubeMetadataEventType_UNSET,
					},
				},
			},
			expectedPodServices: map[string][]string{
				"test-namespace/pod2": {"svc2"},
			},
			expectedNamespaces: map[string]namespaceMetadata{
				"ns2": {labels: map[string]string{"l1": "v2"}},
			},
		},
		{
			name: "empty keepalive",
			initialPodServices: map[string][]string{
				"test-namespace/pod1": {"svc1"},
			},
			initialNamespaces: map[string]namespaceMetadata{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
			initialActive: true,
			response:      &pb.KubeMetadataStreamResponse{},
			expectedPodServices: map[string][]string{
				"test-namespace/pod1": {"svc1"},
			},
			expectedNamespaces: map[string]namespaceMetadata{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sc := &streamClient{
				podServices: test.initialPodServices,
				namespaces:  test.initialNamespaces,
				active:      test.initialActive,
			}

			sc.applyResponse(test.response)

			assert.True(t, sc.active)
			assert.Equal(t, test.expectedPodServices, sc.podServices)
			assert.Equal(t, test.expectedNamespaces, sc.namespaces)
		})
	}
}

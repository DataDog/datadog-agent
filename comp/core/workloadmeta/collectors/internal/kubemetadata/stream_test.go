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

func TestUpdateMappings(t *testing.T) {
	tests := []struct {
		name          string
		initialState  map[string][]string
		initialActive bool
		response      *pb.KubeMetadataStreamResponse
		expectedState map[string][]string
	}{
		{
			name:          "full state with empty initial state",
			initialState:  nil,
			initialActive: false,
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
			},
			expectedState: map[string][]string{
				"test-namespace-1/pod1": {"svc1", "svc2"},
				"test-namespace-2/pod2": {"svc3"},
			},
		},
		{
			name: "full state response replaces existing data",
			initialState: map[string][]string{
				"test-namespace/old-pod": {"old-svc"},
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
			},
			expectedState: map[string][]string{
				"test-namespace/new-pod": {"new-svc"},
			},
		},
		{
			name: "incremental set",
			initialState: map[string][]string{
				"test-namespace/pod1": {"svc1"},
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
			},
			expectedState: map[string][]string{
				"test-namespace/pod1": {"svc1", "svc2"},
				"test-namespace/pod2": {"svc3"},
			},
		},
		{
			name: "unset",
			initialState: map[string][]string{
				"test-namespace/pod1": {"svc1"},
				"test-namespace/pod2": {"svc2"},
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
			},
			expectedState: map[string][]string{
				"test-namespace/pod2": {"svc2"},
			},
		},
		{
			name: "empty keepalive",
			initialState: map[string][]string{
				"test-namespace/pod1": {"svc1"},
			},
			initialActive: true,
			response:      &pb.KubeMetadataStreamResponse{},
			expectedState: map[string][]string{ // Shouldn't change anything
				"test-namespace/pod1": {"svc1"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sc := &streamClient{
				podServices: test.initialState,
				active:      test.initialActive,
			}

			sc.updateMappings(test.response)

			assert.True(t, sc.active)
			assert.Equal(t, test.expectedState, sc.podServices)
		})
	}
}

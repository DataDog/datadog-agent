// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/sets"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestBundleToSnapshot(t *testing.T) {
	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("ns1", "pod1", "svc1", "svc2")
	bundle.Services.Set("ns2", "pod2", "svc3", "svc4")

	snapshot := bundleToSnapshot(bundle)

	assert.Len(t, snapshot, 2)

	actual := snapshot["ns1/pod1"]
	expected := podServiceEntry{
		namespace: "ns1",
		podName:   "pod1",
		services:  sets.New("svc1", "svc2"),
	}
	assert.Equal(t, expected, actual)

	actual = snapshot["ns2/pod2"]
	expected = podServiceEntry{
		namespace: "ns2",
		podName:   "pod2",
		services:  sets.New("svc3", "svc4"),
	}
	assert.Equal(t, expected, actual)
}

func TestComputeDiff(t *testing.T) {
	tests := []struct {
		name     string
		old      map[string]podServiceEntry
		current  map[string]podServiceEntry
		expected []*pb.PodServiceMapping
	}{
		{
			name: "no changes",
			old: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
			},
			current: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
			},
			expected: nil,
		},
		{
			name: "new pod",
			old:  map[string]podServiceEntry{},
			current: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
			},
			expected: []*pb.PodServiceMapping{
				{
					Namespace:    "default",
					PodName:      "pod1",
					ServiceNames: []string{"svc1"},
					Type:         pb.KubeMetadataEventType_SET,
				},
			},
		},
		{
			name: "removed pod",
			old: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
			},
			current: map[string]podServiceEntry{},
			expected: []*pb.PodServiceMapping{
				{
					Namespace: "default",
					PodName:   "pod1",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
			},
		},
		{
			name: "services changed",
			old: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
			},
			current: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1", "svc2"),
				},
			},
			expected: []*pb.PodServiceMapping{
				{
					Namespace:    "default",
					PodName:      "pod1",
					ServiceNames: []string{"svc1", "svc2"},
					Type:         pb.KubeMetadataEventType_SET,
				},
			},
		},
		{
			name: "current is nil unsets all",
			old: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
				"default/pod2": {
					namespace: "default",
					podName:   "pod2",
					services:  sets.New("svc2"),
				},
			},
			current: nil,
			expected: []*pb.PodServiceMapping{
				{
					Namespace: "default",
					PodName:   "pod1",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
				{
					Namespace: "default",
					PodName:   "pod2",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
			},
		},
		{
			name: "1 pod added and 1 pod removed",
			old: map[string]podServiceEntry{
				"default/pod1": {
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
				"default/pod2": {
					namespace: "default",
					podName:   "pod2",
					services:  sets.New("svc2"),
				},
			},
			current: map[string]podServiceEntry{
				"default/pod1": { // Unchanged
					namespace: "default",
					podName:   "pod1",
					services:  sets.New("svc1"),
				},
				"default/pod3": { // Added
					namespace: "default",
					podName:   "pod3",
					services:  sets.New("svc2"),
				},
			},
			expected: []*pb.PodServiceMapping{
				{
					Namespace: "default",
					PodName:   "pod2",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
				{
					Namespace:    "default",
					PodName:      "pod3",
					ServiceNames: []string{"svc2"},
					Type:         pb.KubeMetadataEventType_SET,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diff := computeDiff(test.old, test.current)
			assert.ElementsMatch(t, test.expected, diff)
		})
	}
}

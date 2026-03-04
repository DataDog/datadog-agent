// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestStart(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	srv := NewKubeMetadataStreamServer(nil, wmetaMock)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	ch := srv.subscribeToNamespaceEvents("node1")
	srv.Start(ctx)

	namespace := "ns1"
	namespaceKubeMetadataID := string(util.GenerateKubeMetadataEntityID("", "namespaces", "", namespace))

	wmetaMock.Set(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   namespaceKubeMetadataID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        namespace,
			Labels:      map[string]string{"l1": "v1"},
			Annotations: map[string]string{"a1": "v2"},
		},
		GVR: &schema.GroupVersionResource{Resource: "namespaces"},
	})

	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for set notification")
	}

	snapshot := srv.buildNamespacesSnapshot()
	assert.Equal(t, map[string]namespaceEntry{
		namespace: {
			labels:      map[string]string{"l1": "v1"},
			annotations: map[string]string{"a1": "v2"},
		},
	}, snapshot)

	wmetaMock.Unset(&workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   namespaceKubeMetadataID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: namespace,
		},
		GVR: &schema.GroupVersionResource{Resource: "namespaces"},
	})

	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for unset notification")
	}

	assert.Empty(t, srv.buildNamespacesSnapshot())
}

func TestBundleToPodServiceMappingsSnapshot(t *testing.T) {
	bundle := apiserver.NewMetadataMapperBundle()
	bundle.Services.Set("ns1", "pod1", "svc1", "svc2")
	bundle.Services.Set("ns2", "pod2", "svc3", "svc4")

	snapshot := bundleToPodServiceMappingsSnapshot(bundle)

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

func TestComputePodServiceMappingsDiff(t *testing.T) {
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
			diff := computePodServiceMappingsDiff(test.old, test.current)
			assert.ElementsMatch(t, test.expected, diff)
		})
	}
}

func TestComputeNamespacesDiff(t *testing.T) {
	tests := []struct {
		name     string
		old      map[string]namespaceEntry
		current  map[string]namespaceEntry
		expected []*pb.NamespaceMetadata
	}{
		{
			name: "no changes",
			old: map[string]namespaceEntry{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
			current: map[string]namespaceEntry{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
			expected: nil,
		},
		{
			name: "new namespace",
			old:  map[string]namespaceEntry{},
			current: map[string]namespaceEntry{
				"ns1": {
					labels:      map[string]string{"l1": "v1"},
					annotations: map[string]string{"a1": "v2"},
				},
			},
			expected: []*pb.NamespaceMetadata{
				{
					Namespace:   "ns1",
					Labels:      map[string]string{"l1": "v1"},
					Annotations: map[string]string{"a1": "v2"},
					Type:        pb.KubeMetadataEventType_SET,
				},
			},
		},
		{
			name: "removed namespace",
			old: map[string]namespaceEntry{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
			current: map[string]namespaceEntry{},
			expected: []*pb.NamespaceMetadata{
				{
					Namespace: "ns1",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
			},
		},
		{
			name: "labels changed",
			old: map[string]namespaceEntry{
				"ns1": {labels: map[string]string{"l1": "v1"}},
			},
			current: map[string]namespaceEntry{
				"ns1": {labels: map[string]string{"l1": "v2"}},
			},
			expected: []*pb.NamespaceMetadata{
				{
					Namespace: "ns1",
					Labels:    map[string]string{"l1": "v2"},
					Type:      pb.KubeMetadataEventType_SET,
				},
			},
		},
		{
			name: "current is nil unsets all",
			old: map[string]namespaceEntry{
				"ns1": {labels: map[string]string{"l1": "v1"}},
				"ns2": {labels: map[string]string{"l1": "v2"}},
			},
			current: nil,
			expected: []*pb.NamespaceMetadata{
				{
					Namespace: "ns1",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
				{
					Namespace: "ns2",
					Type:      pb.KubeMetadataEventType_UNSET,
				},
			},
		},
		{
			name: "annotations changed",
			old: map[string]namespaceEntry{
				"ns1": {
					labels:      map[string]string{"l1": "v1"},
					annotations: map[string]string{"a1": "v2"},
				},
			},
			current: map[string]namespaceEntry{
				"ns1": {
					labels:      map[string]string{"l1": "v1"},
					annotations: map[string]string{"a1": "v3"},
				},
			},
			expected: []*pb.NamespaceMetadata{
				{
					Namespace:   "ns1",
					Labels:      map[string]string{"l1": "v1"},
					Annotations: map[string]string{"a1": "v3"},
					Type:        pb.KubeMetadataEventType_SET,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			diff := computeNamespacesDiff(test.old, test.current)
			assert.ElementsMatch(t, test.expected, diff)
		})
	}
}

func TestFullStateResponse(t *testing.T) {
	pods := map[string]podServiceEntry{
		"ns1/pod1": {
			namespace: "ns1",
			podName:   "pod1",
			services:  sets.New("svc1"),
		},
	}
	namespaces := map[string]namespaceEntry{
		"ns1": {
			labels:      map[string]string{"l1": "v1"},
			annotations: map[string]string{"a1": "v2"},
		},
	}

	resp := fullStateResponse(pods, namespaces)

	expected := &pb.KubeMetadataStreamResponse{
		IsFullState: true,
		Mappings: []*pb.PodServiceMapping{
			{
				Namespace:    "ns1",
				PodName:      "pod1",
				ServiceNames: []string{"svc1"},
				Type:         pb.KubeMetadataEventType_SET,
			},
		},
		NamespaceMetadata: []*pb.NamespaceMetadata{
			{
				Namespace:   "ns1",
				Labels:      map[string]string{"l1": "v1"},
				Annotations: map[string]string{"a1": "v2"},
				Type:        pb.KubeMetadataEventType_SET,
			},
		},
	}

	assert.True(t, proto.Equal(expected, resp))
}

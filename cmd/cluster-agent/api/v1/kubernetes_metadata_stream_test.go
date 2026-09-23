// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
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

func TestSubscribeToNamespaceEvents(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)

	// Two subscriptions for the same node. Both must receive notifications.
	ch1 := srv.subscribeToNamespaceEvents("node1")
	defer srv.unsubscribeFromNamespaceEvents("node1", ch1)
	ch2 := srv.subscribeToNamespaceEvents("node1")
	defer srv.unsubscribeFromNamespaceEvents("node1", ch2)

	srv.processWmetaEvents(testNamespaceSetEvents())

	assertNotified(t, "first", ch1)
	assertNotified(t, "second", ch2)
}

func TestUnsubscribeFromNamespaceEvents(t *testing.T) {
	t.Run("unsubscribing the older subscriber leaves the newer one working", func(t *testing.T) {
		srv := NewKubeMetadataStreamServer(nil, nil)
		olderCh := srv.subscribeToNamespaceEvents("node1")
		newerCh := srv.subscribeToNamespaceEvents("node1")

		srv.unsubscribeFromNamespaceEvents("node1", olderCh)
		srv.processWmetaEvents(testNamespaceSetEvents())

		assertNotified(t, "newer", newerCh)
		assertNotNotified(t, "older", olderCh)
	})

	t.Run("unsubscribing the newer subscriber leaves the older one working", func(t *testing.T) {
		srv := NewKubeMetadataStreamServer(nil, nil)
		olderCh := srv.subscribeToNamespaceEvents("node1")
		newerCh := srv.subscribeToNamespaceEvents("node1")

		srv.unsubscribeFromNamespaceEvents("node1", newerCh)
		srv.processWmetaEvents(testNamespaceSetEvents())

		assertNotified(t, "older", olderCh)
		assertNotNotified(t, "newer", newerCh)
	})

	t.Run("unsubscribing an unknown channel is a no-op", func(t *testing.T) {
		srv := NewKubeMetadataStreamServer(nil, nil)
		olderCh := srv.subscribeToNamespaceEvents("node1")
		newerCh := srv.subscribeToNamespaceEvents("node1")

		unknown := make(chan struct{}, 1)
		srv.unsubscribeFromNamespaceEvents("node1", unknown)
		srv.processWmetaEvents(testNamespaceSetEvents())

		assertNotified(t, "older", olderCh)
		assertNotified(t, "newer", newerCh)
	})
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

func TestProcessKueueQueueEvents(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesKueueQueue{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueQueue,
					ID:   "localqueue/ns1/local-a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "local-a",
					Namespace:   "ns1",
					Labels:      map[string]string{"team": "batch"},
					Annotations: map[string]string{"owner": "team-a"},
				},
				QueueType:        workloadmeta.KueueLocalQueue,
				ClusterQueueName: "cluster-a",
			},
		},
	})

	snapshot := srv.buildKueueQueuesSnapshot()
	entry := snapshot["localqueue/ns1/local-a"]
	assert.Equal(t, "ns1", entry.namespace)
	assert.Equal(t, "local-a", entry.name)
	assert.Equal(t, workloadmeta.KueueLocalQueue, entry.queueType)
	assert.Equal(t, "cluster-a", entry.clusterQueueName)
	assert.Equal(t, map[string]string{"team": "batch"}, entry.labels)
	assert.Equal(t, map[string]string{"owner": "team-a"}, entry.annotations)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesKueueQueue{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueQueue,
					ID:   "clusterqueue//cluster-a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:   "cluster-a",
					Labels: map[string]string{"tier": "gold"},
				},
				QueueType:        workloadmeta.KueueClusterQueue,
				ClusterQueueName: "cluster-a",
			},
		},
	})
	entry = srv.buildKueueQueuesSnapshot()["clusterqueue//cluster-a"]
	assert.Equal(t, "cluster-a", entry.name)
	assert.Equal(t, workloadmeta.KueueClusterQueue, entry.queueType)
	assert.Equal(t, map[string]string{"tier": "gold"}, entry.labels)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesKueueQueue{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueQueue,
					ID:   "localqueue/ns1/local-a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "local-a",
					Namespace: "ns1",
				},
				QueueType: workloadmeta.KueueLocalQueue,
			},
		},
		{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesKueueQueue{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueQueue,
					ID:   "clusterqueue//cluster-a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "cluster-a",
				},
				QueueType: workloadmeta.KueueClusterQueue,
			},
		},
	})

	assert.Empty(t, srv.buildKueueQueuesSnapshot())
}

func TestComputeKueueQueueDiff(t *testing.T) {
	old := map[string]kueueQueueEntry{
		"localqueue/ns1/local-a": {
			namespace:   "ns1",
			name:        "local-a",
			queueType:   workloadmeta.KueueLocalQueue,
			labels:      map[string]string{"queue": "old"},
			annotations: map[string]string{"owner": "team-a"},
		},
		"localqueue/ns1/local-b": {
			namespace: "ns1",
			name:      "local-b",
			queueType: workloadmeta.KueueLocalQueue,
		},
	}
	current := map[string]kueueQueueEntry{
		"localqueue/ns1/local-a": {
			namespace:   "ns1",
			name:        "local-a",
			queueType:   workloadmeta.KueueLocalQueue,
			labels:      map[string]string{"queue": "new"},
			annotations: map[string]string{"owner": "team-a"},
		},
	}

	diff := computeKueueQueueDiff(old, current)

	assert.ElementsMatch(t, []*pb.KueueQueue{
		{
			Namespace:   "ns1",
			Name:        "local-a",
			QueueType:   pb.KueueQueueType_LOCAL_QUEUE,
			Labels:      map[string]string{"queue": "new"},
			Annotations: map[string]string{"owner": "team-a"},
			Type:        pb.KubeMetadataEventType_SET,
		},
		{
			Namespace: "ns1",
			Name:      "local-b",
			QueueType: pb.KueueQueueType_LOCAL_QUEUE,
			Type:      pb.KubeMetadataEventType_UNSET,
		},
	}, diff)
}

func TestProcessKueueResourceFlavorEvents(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesKueueResourceFlavor{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueResourceFlavor,
					ID:   "a100",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "a100",
					Labels:      map[string]string{"flavor_class": "gpu"},
					Annotations: map[string]string{"owner": "team-a"},
				},
				NodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
			},
		},
	})

	snapshot := srv.buildKueueResourceFlavorsSnapshot()
	entry := snapshot["a100"]
	assert.Equal(t, "a100", entry.name)
	assert.Equal(t, map[string]string{"flavor_class": "gpu"}, entry.labels)
	assert.Equal(t, map[string]string{"owner": "team-a"}, entry.annotations)
	assert.Equal(t, map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"}, entry.nodeAffinityLabels)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesKueueResourceFlavor{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueResourceFlavor,
					ID:   "a100",
				},
			},
		},
	})

	assert.Empty(t, srv.buildKueueResourceFlavorsSnapshot())
}

func TestComputeKueueResourceFlavorDiff(t *testing.T) {
	old := map[string]kueueResourceFlavorEntry{
		"a100": {
			name:               "a100",
			labels:             map[string]string{"tier": "old"},
			annotations:        map[string]string{"owner": "team-a"},
			nodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "old"},
		},
		"h100": {
			name: "h100",
		},
	}
	current := map[string]kueueResourceFlavorEntry{
		"a100": {
			name:               "a100",
			labels:             map[string]string{"tier": "new"},
			annotations:        map[string]string{"owner": "team-a"},
			nodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
		},
	}

	diff := computeKueueResourceFlavorDiff(old, current)

	assert.ElementsMatch(t, []*pb.KueueResourceFlavor{
		{
			Name:               "a100",
			Labels:             map[string]string{"tier": "new"},
			Annotations:        map[string]string{"owner": "team-a"},
			NodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
			Type:               pb.KubeMetadataEventType_SET,
		},
		{
			Name: "h100",
			Type: pb.KubeMetadataEventType_UNSET,
		},
	}, diff)
}

func TestProcessKueueWorkloadEvents(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesKueueWorkload{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueWorkload,
					ID:   "team-a/job-sample",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "job-sample",
					Namespace: "team-a",
					Labels: map[string]string{
						"team":  "eng",
						"owner": "alice",
					},
					Annotations: map[string]string{
						"cost-center": "1234",
					},
				},
				QueueName:        "gpu",
				ClusterQueueName: "team-a-gpu",
				PodSetAssignments: []workloadmeta.KueuePodSetAssignment{
					{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
				},
			},
		},
	})

	snapshot := srv.buildKueueWorkloadsSnapshot()
	entry := snapshot["team-a/job-sample"]
	assert.Equal(t, "team-a", entry.namespace)
	assert.Equal(t, "job-sample", entry.name)
	assert.Equal(t, "gpu", entry.queueName)
	assert.Equal(t, "team-a-gpu", entry.clusterQueueName)
	assert.Equal(t, map[string]string{
		"team":  "eng",
		"owner": "alice",
	}, entry.labels)
	assert.Equal(t, map[string]string{
		"cost-center": "1234",
	}, entry.annotations)
	assert.Equal(t, []kueuePodSetAssignmentEntry{
		{name: "main", flavors: map[string]string{"nvidia.com/gpu": "a100"}},
	}, entry.podSetAssignments)

	srv.processWmetaEvents([]workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeUnset,
			Entity: &workloadmeta.KubernetesKueueWorkload{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueWorkload,
					ID:   "team-a/job-sample",
				},
			},
		},
	})

	assert.Empty(t, srv.buildKueueWorkloadsSnapshot())
}

func TestComputeKueueWorkloadDiff(t *testing.T) {
	old := map[string]kueueWorkloadEntry{
		"team-a/job-a": {
			namespace:        "team-a",
			name:             "job-a",
			queueName:        "gpu",
			clusterQueueName: "old-cq",
			labels:           map[string]string{"team": "old"},
			podSetAssignments: []kueuePodSetAssignmentEntry{
				{name: "main", flavors: map[string]string{"nvidia.com/gpu": "old"}},
			},
		},
		"team-a/job-b": {
			namespace: "team-a",
			name:      "job-b",
		},
	}
	current := map[string]kueueWorkloadEntry{
		"team-a/job-a": {
			namespace:        "team-a",
			name:             "job-a",
			queueName:        "gpu",
			clusterQueueName: "team-a-gpu",
			labels:           map[string]string{"team": "new"},
			podSetAssignments: []kueuePodSetAssignmentEntry{
				{name: "main", flavors: map[string]string{"nvidia.com/gpu": "a100"}},
			},
		},
	}

	diff := computeKueueWorkloadDiff(old, current)

	assert.ElementsMatch(t, []*pb.KueueWorkload{
		{
			Namespace:    "team-a",
			Name:         "job-a",
			Queue:        "gpu",
			ClusterQueue: "team-a-gpu",
			Labels:       map[string]string{"team": "new"},
			PodSetAssignments: []*pb.KueuePodSetAssignment{
				{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
			},
			Type: pb.KubeMetadataEventType_SET,
		},
		{
			Namespace: "team-a",
			Name:      "job-b",
			Type:      pb.KubeMetadataEventType_UNSET,
		},
	}, diff)
}

func TestFullStateResponse(t *testing.T) {
	pods := map[string]podServiceEntry{
		"ns1/pod1": {
			namespace: "ns1",
			podName:   "pod1",
			services:  sets.New("svc1"),
		},
	}
	metadata := newMetadataSnapshot()
	metadata.namespaces["ns1"] = namespaceEntry{
		labels:      map[string]string{"l1": "v1"},
		annotations: map[string]string{"a1": "v2"},
	}
	metadata.kueueResourceFlavors["a100"] = kueueResourceFlavorEntry{
		name:               "a100",
		nodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
	}
	metadata.kueueWorkloads["team-a/job-sample"] = kueueWorkloadEntry{
		namespace:        "team-a",
		name:             "job-sample",
		queueName:        "gpu",
		clusterQueueName: "team-a-gpu",
		labels:           map[string]string{"team": "eng"},
		podSetAssignments: []kueuePodSetAssignmentEntry{
			{name: "main", flavors: map[string]string{"nvidia.com/gpu": "a100"}},
		},
	}

	resp := fullStateResponse(pods, metadata)

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
		KueueResourceFlavors: []*pb.KueueResourceFlavor{
			{
				Name:               "a100",
				NodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
				Type:               pb.KubeMetadataEventType_SET,
			},
		},
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace:    "team-a",
				Name:         "job-sample",
				Queue:        "gpu",
				ClusterQueue: "team-a-gpu",
				Labels:       map[string]string{"team": "eng"},
				PodSetAssignments: []*pb.KueuePodSetAssignment{
					{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
				},
				Type: pb.KubeMetadataEventType_SET,
			},
		},
	}

	assert.True(t, proto.Equal(expected, resp))
}

func testNamespaceSetEvents() []workloadmeta.Event {
	return []workloadmeta.Event{
		{
			Type: workloadmeta.EventTypeSet,
			Entity: &workloadmeta.KubernetesMetadata{
				EntityMeta: workloadmeta.EntityMeta{
					Name:   "ns1",
					Labels: map[string]string{"l1": "v1"},
				},
			},
		},
	}
}

func assertNotified(t *testing.T, name string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(1 * time.Second):
		t.Fatalf("%s subscriber was not notified", name)
	}
}

func assertNotNotified(t *testing.T, name string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s subscriber was notified but should not have been", name)
	case <-time.After(50 * time.Millisecond):
	}
}

func deploymentEntity(id string, kinds sets.Set[string]) *workloadmeta.KubernetesDeployment {
	return &workloadmeta.KubernetesDeployment{
		EntityID:        workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: id},
		AutoscalerKinds: kinds,
	}
}

func TestProcessDeploymentEvents(t *testing.T) {
	app := kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: "app"}
	s := newMetadataSnapshot()
	changed := func(eventType workloadmeta.EventType, deployment *workloadmeta.KubernetesDeployment) bool {
		_, c := s.processDeploymentEvent(eventType, deployment)
		return c
	}

	// A deployment without autoscalers is not tracked, and waking every node's
	// stream for it would be wasted work.
	assert.False(t, changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", nil)))
	assert.Empty(t, s.workloadAutoscalers)

	assert.True(t, changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", sets.New("hpa"))))
	assert.Equal(t, sets.New("hpa"), s.workloadAutoscalers[app])

	// Same set again, e.g. the deployment reflector reporting an unrelated
	// change: no wakeup.
	assert.False(t, changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", sets.New("hpa"))))

	assert.True(t, changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", sets.New("hpa", "vpa"))))
	assert.Equal(t, sets.New("hpa", "vpa"), s.workloadAutoscalers[app])

	// The stored set must not alias the entity's: workloadmeta owns that one.
	kinds := sets.New("wpa")
	changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", kinds))
	kinds.Insert("vpa")
	assert.Equal(t, sets.New("wpa"), s.workloadAutoscalers[app])

	// Cleared through a Set with an empty set, as the autoscaler collector does.
	assert.True(t, changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", sets.New[string]())))
	assert.Empty(t, s.workloadAutoscalers)

	// An Unset for a deleted Deployment carries only its ID: the workload must
	// still be identified, from the ID.
	changed(workloadmeta.EventTypeSet, deploymentEntity("ns/app", sets.New("keda")))
	assert.True(t, changed(workloadmeta.EventTypeUnset, deploymentEntity("ns/app", nil)))
	assert.Empty(t, s.workloadAutoscalers)
	assert.False(t, changed(workloadmeta.EventTypeUnset, deploymentEntity("ns/app", nil)), "unset of an untracked deployment")
}

func TestComputeWorkloadAutoscalersDiff(t *testing.T) {
	target := func(name string) kubernetes.WorkloadTarget {
		return kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: name}
	}
	old := map[kubernetes.WorkloadTarget]sets.Set[string]{
		target("unchanged"): sets.New("hpa"),
		target("changed"):   sets.New("hpa"),
		target("removed"):   sets.New("vpa"),
	}
	current := map[kubernetes.WorkloadTarget]sets.Set[string]{
		target("unchanged"): sets.New("hpa"),
		target("changed"):   sets.New("vpa", "hpa", "dpa"),
		target("added"):     sets.New("keda"),
	}

	assert.ElementsMatch(t, []*pb.WorkloadAutoscalers{
		// Sorted on the wire, whatever the set's iteration order.
		{Namespace: "ns", Kind: "Deployment", Name: "changed", AutoscalerKinds: []string{"dpa", "hpa", "vpa"}, Type: pb.KubeMetadataEventType_SET},
		{Namespace: "ns", Kind: "Deployment", Name: "added", AutoscalerKinds: []string{"keda"}, Type: pb.KubeMetadataEventType_SET},
		{Namespace: "ns", Kind: "Deployment", Name: "removed", AutoscalerKinds: []string{}, Type: pb.KubeMetadataEventType_UNSET},
	}, computeWorkloadAutoscalersDiff(old, current))
}

// TestStreamWorkloadAutoscalersThroughRealStore replays what the Cluster Agent
// really produces: a Deployment with two workloadmeta sources (its reflector
// and the autoscaler collector), and a clear done as Set(empty) then Unset.
// The server must follow the merged entity, stay quiet on unrelated
// Deployment updates, and see the clear.
func TestStreamWorkloadAutoscalersThroughRealStore(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	srv := NewKubeMetadataStreamServer(nil, wmetaMock)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	notified := srv.subscribeToNamespaceEvents("node1")
	srv.Start(ctx)

	app := kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: "app"}
	reflectorEntity := func(labels map[string]string) *workloadmeta.KubernetesDeployment {
		return &workloadmeta.KubernetesDeployment{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: "ns/app"},
			EntityMeta: workloadmeta.EntityMeta{Name: "app", Namespace: "ns", Labels: labels},
		}
	}
	notify := func(eventType workloadmeta.EventType, source workloadmeta.Source, entity workloadmeta.Entity) {
		wmetaMock.Notify([]workloadmeta.CollectorEvent{{Type: eventType, Source: source, Entity: entity}})
	}
	expectNotified := func(what string) {
		t.Helper()
		select {
		case <-notified:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for a notification: %s", what)
		}
	}
	expectQuiet := func(what string) {
		t.Helper()
		select {
		case <-notified:
			t.Fatalf("unexpected notification: %s", what)
		case <-time.After(300 * time.Millisecond):
		}
	}

	notify(workloadmeta.EventTypeSet, workloadmeta.SourceKubeAPIServer, reflectorEntity(nil))
	expectQuiet("deployment without autoscalers")

	notify(workloadmeta.EventTypeSet, workloadmeta.SourceKubeAutoscalers, deploymentEntity("ns/app", sets.New("hpa")))
	expectNotified("autoscaler added")
	assert.Equal(t, sets.New("hpa"), srv.buildMetadataSnapshot().workloadAutoscalers[app])

	notify(workloadmeta.EventTypeSet, workloadmeta.SourceKubeAPIServer, reflectorEntity(map[string]string{"team": "platform"}))
	expectQuiet("unrelated deployment update")

	// The clear, exactly as the autoscaler collector emits it.
	wmetaMock.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceKubeAutoscalers, Entity: deploymentEntity("ns/app", sets.New[string]())},
		{Type: workloadmeta.EventTypeUnset, Source: workloadmeta.SourceKubeAutoscalers, Entity: deploymentEntity("ns/app", sets.New[string]())},
	})
	expectNotified("last autoscaler removed")
	assert.Empty(t, srv.buildMetadataSnapshot().workloadAutoscalers)
}

// StatefulSets and Argo Rollouts arrive as KubernetesMetadata, the same entity
// kind as namespaces. They must feed the workload index and never be mistaken
// for a namespace.
func TestStreamWorkloadMetadataThroughRealStore(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	srv := NewKubeMetadataStreamServer(nil, wmetaMock)
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	notified := srv.subscribeToNamespaceEvents("node1")
	srv.Start(ctx)
	waitNotified := func(what string) {
		t.Helper()
		select {
		case <-notified:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for a notification: %s", what)
		}
	}

	db := kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: "ns", Name: "db"}
	dbEntity := func(kinds sets.Set[string]) *workloadmeta.KubernetesMetadata {
		entity, ok := util.WorkloadMetadataEntity(db)
		require.True(t, ok)
		entity.AutoscalerKinds = kinds
		return entity
	}

	wmetaMock.Notify([]workloadmeta.CollectorEvent{{
		Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceKubeAutoscalers, Entity: dbEntity(sets.New("hpa")),
	}})
	waitNotified("statefulset autoscaler added")

	snapshot := srv.buildMetadataSnapshot()
	assert.Equal(t, sets.New("hpa"), snapshot.workloadAutoscalers[db])
	assert.NotContains(t, snapshot.namespaces, "db", "a StatefulSet must not be recorded as a namespace")

	// A real namespace still takes the namespace path.
	wmetaMock.Set(&workloadmeta.KubernetesMetadata{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesMetadata, ID: string(util.GenerateKubeMetadataEntityID("", "namespaces", "", "ns"))},
		EntityMeta: workloadmeta.EntityMeta{Name: "ns", Labels: map[string]string{"team": "data"}},
		GVR:        &schema.GroupVersionResource{Version: "v1", Resource: "namespaces"},
	})
	waitNotified("namespace added")
	assert.Contains(t, srv.buildMetadataSnapshot().namespaces, "ns")

	// Cleared as the autoscaler collector does it.
	wmetaMock.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceKubeAutoscalers, Entity: dbEntity(sets.New[string]())},
		{Type: workloadmeta.EventTypeUnset, Source: workloadmeta.SourceKubeAutoscalers, Entity: dbEntity(sets.New[string]())},
	})
	waitNotified("statefulset autoscaler removed")
	assert.Empty(t, srv.buildMetadataSnapshot().workloadAutoscalers)
}

// TestPerNodeWorkloadFiltering goes through the real workloadmeta store: pods
// from the Kubernetes API server source say where each workload runs, and each
// node must receive only the autoscalers of its own workloads, and be woken up
// only when those change.
func TestPerNodeWorkloadFiltering(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	synced := make(chan struct{})
	waitSynced := func(ctx context.Context) bool {
		select {
		case <-synced:
			return true
		case <-ctx.Done():
			return false
		}
	}
	srv := NewKubeMetadataStreamServer(nil, wmetaMock, WithPerNodeWorkloadFiltering(waitSynced))
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	notified := map[string]<-chan struct{}{}
	for _, node := range []string{"node-a", "node-b", "node-c"} {
		notified[node] = srv.subscribeToNamespaceEvents(node)
	}
	srv.Start(ctx)

	expectNotified := func(node, what string) {
		t.Helper()
		select {
		case <-notified[node]:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: timed out waiting for a notification: %s", node, what)
		}
	}
	expectQuiet := func(node, what string) {
		t.Helper()
		select {
		case <-notified[node]:
			t.Fatalf("%s: unexpected notification: %s", node, what)
		case <-time.After(300 * time.Millisecond):
		}
	}
	drain := func() {
		for _, ch := range notified {
			for drained := false; !drained; {
				select {
				case <-ch:
				case <-time.After(300 * time.Millisecond):
					drained = true
				}
			}
		}
	}
	workloads := func(node string) []string {
		var names []string
		for target := range srv.buildMetadataSnapshotForNode(node).workloadAutoscalers {
			names = append(names, target.Name)
		}
		sort.Strings(names)
		return names
	}
	setPod := func(uid, node, replicaSet string) {
		wmetaMock.Notify([]workloadmeta.CollectorEvent{{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: uid},
				EntityMeta: workloadmeta.EntityMeta{Name: uid, Namespace: "ns"},
				Owners:     []workloadmeta.KubernetesPodOwner{{Kind: kubernetes.ReplicaSetKind, Name: replicaSet}},
				NodeName:   node,
			},
		}})
	}
	setKinds := func(deployment string, kinds sets.Set[string]) {
		wmetaMock.Notify([]workloadmeta.CollectorEvent{{
			Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceKubeAutoscalers, Entity: deploymentEntity("ns/"+deployment, kinds),
		}})
	}

	setKinds("app", sets.New("hpa"))
	setKinds("other", sets.New("vpa"))
	setPod("app-1", "node-a", "app-7d9f8b6c5d")
	setPod("other-1", "node-b", "other-5c4b6d8f7")
	drain()

	// Before the pod collection has synced, placement may be incomplete: send
	// everything, as without filtering, rather than risk starving a node.
	assert.Equal(t, []string{"app", "other"}, workloads("node-a"))

	close(synced)
	for _, node := range []string{"node-a", "node-b", "node-c"} {
		expectNotified(node, "switch to per-node filtering")
	}
	assert.Equal(t, []string{"app"}, workloads("node-a"))
	assert.Equal(t, []string{"other"}, workloads("node-b"))
	assert.Empty(t, workloads("node-c"))

	// A pod not bound yet does not count; once bound, only its node hears it.
	setPod("app-2", "", "app-7d9f8b6c5d")
	expectQuiet("node-b", "unbound pod")
	setPod("app-2", "node-b", "app-7d9f8b6c5d")
	expectNotified("node-b", "app now has a pod on node-b")
	expectQuiet("node-a", "app was already on node-a")
	assert.Equal(t, []string{"app", "other"}, workloads("node-b"))

	// A kinds change wakes only the nodes hosting the workload.
	setKinds("app", sets.New("hpa", "vpa"))
	expectNotified("node-a", "app kinds changed")
	expectNotified("node-b", "app kinds changed")
	expectQuiet("node-c", "app has no pod on node-c")

	// The last pod of a workload leaving a node: that node only, and the
	// workload leaves its snapshot, which the stream sends as an UNSET.
	before := srv.buildMetadataSnapshotForNode("node-a")
	wmetaMock.Notify([]workloadmeta.CollectorEvent{{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceKubeAPIServer,
		Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "app-1"}},
	}})
	expectNotified("node-a", "app's last pod left node-a")
	expectQuiet("node-b", "app still on node-b")
	assert.Empty(t, workloads("node-a"))
	diff := computeWorkloadAutoscalersDiff(before.workloadAutoscalers, srv.buildMetadataSnapshotForNode("node-a").workloadAutoscalers)
	require.Len(t, diff, 1)
	assert.Equal(t, pb.KubeMetadataEventType_UNSET, diff[0].Type)
	assert.Equal(t, "app", diff[0].Name)

	// Pods of a workload without autoscalers move freely: nobody's view changes.
	setPod("plain-1", "node-c", "plain-6b5d4c8f2")
	expectQuiet("node-c", "non-autoscaled workload")
}

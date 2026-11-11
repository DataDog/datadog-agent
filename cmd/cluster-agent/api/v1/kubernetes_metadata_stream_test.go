// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/grpc/metadata"
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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
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
				PodTemplateLabels: map[string]string{
					"team": "ccm-allocation",
				},
				PodTemplateAnnotations: map[string]string{
					"workload-owner": "gpu-monitoring",
				},
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
	assert.Equal(t, map[string]string{
		"team": "ccm-allocation",
	}, entry.podTemplateLabels)
	assert.Equal(t, map[string]string{
		"workload-owner": "gpu-monitoring",
	}, entry.podTemplateAnnotations)
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
			namespace:         "team-a",
			name:              "job-a",
			queueName:         "gpu",
			clusterQueueName:  "team-a-gpu",
			labels:            map[string]string{"team": "new"},
			podTemplateLabels: map[string]string{"team": "ccm-allocation"},
			podSetAssignments: []kueuePodSetAssignmentEntry{
				{name: "main", flavors: map[string]string{"nvidia.com/gpu": "a100"}},
			},
		},
	}

	diff := computeKueueWorkloadDiff(old, current)

	assert.ElementsMatch(t, []*pb.KueueWorkload{
		{
			Namespace:         "team-a",
			Name:              "job-a",
			Queue:             "gpu",
			ClusterQueue:      "team-a-gpu",
			Labels:            map[string]string{"team": "new"},
			PodTemplateLabels: map[string]string{"team": "ccm-allocation"},
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

func testKAIPodGroupEvent(eventType workloadmeta.EventType, namespace, name, queue string) workloadmeta.Event {
	return workloadmeta.Event{
		Type: eventType,
		Entity: &workloadmeta.KubernetesKAIPodGroup{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesKAIPodGroup,
				ID:   workloadmeta.GenerateKAIPodGroupEntityID(namespace, name),
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      name,
				Namespace: namespace,
				UID:       "uid-" + name,
			},
			QueueName: queue,
		},
	}
}

func testKAIPodEvent(eventType workloadmeta.EventType, uid, namespace, nodeName, podGroupName string) workloadmeta.Event {
	annotations := map[string]string{}
	if podGroupName != "" {
		annotations[kubernetes.KAIPodGroupAnnotationKey] = podGroupName
	}
	return workloadmeta.Event{
		Type: eventType,
		Entity: &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: uid},
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-" + uid,
				Namespace:   namespace,
				Annotations: annotations,
			},
			NodeName: nodeName,
		},
	}
}

func kaiPodGroupKeys(snapshot map[string]kaiPodGroupEntry) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	return keys
}

func TestProcessKAIPodGroupEvents(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	node1 := srv.subscribeToNamespaceEvents("node1")
	node2 := srv.subscribeToNamespaceEvents("node2")

	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "team-a", "node1", "pg")})
	assertNotified(t, "node1", node1)
	assertNotNotified(t, "node2", node2)
	assert.Empty(t, srv.buildNodeMetadataState("node1").kaiPodGroups, "the PodGroup is not known yet")

	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "pg", "queue-a")})
	assertNotified(t, "node1", node1)
	assertNotNotified(t, "node2", node2)
	assert.Equal(t, map[string]kaiPodGroupEntry{
		"team-a/pg": {namespace: "team-a", name: "pg", queueName: "queue-a", uid: "uid-pg"},
	}, srv.buildNodeMetadataState("node1").kaiPodGroups)

	// An unchanged PodGroup does not wake its node.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "pg", "queue-a")})
	assertNotNotified(t, "node1", node1)

	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "pg", "queue-b")})
	assertNotified(t, "node1", node1)
	assert.Equal(t, "queue-b", srv.buildNodeMetadataState("node1").kaiPodGroups["team-a/pg"].queueName)

	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeUnset, "team-a", "pg", "queue-b")})
	assertNotified(t, "node1", node1)
	assert.Empty(t, srv.buildNodeMetadataState("node1").kaiPodGroups)

	// PodGroups never go through the shared history.
	_, version := srv.buildMetadataSnapshotWithVersion()
	assert.Zero(t, version)
}

func TestKAIPodGroupsPerNode(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	node1 := srv.subscribeToNamespaceEvents("node1")
	node2 := srv.subscribeToNamespaceEvents("node2")
	node3 := srv.subscribeToNamespaceEvents("node3")

	srv.processWmetaEvents([]workloadmeta.Event{
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "gang", "queue-a"),
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "single", "queue-a"),
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-b", "gang", "queue-b"),
	})
	assertNotNotified(t, "node1", node1)

	// A gang spread over two nodes, a single-pod PodGroup, and a PodGroup with
	// the same name in another namespace.
	srv.processWmetaEvents([]workloadmeta.Event{
		testKAIPodEvent(workloadmeta.EventTypeSet, "gang-0", "team-a", "node1", "gang"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "gang-1", "team-a", "node1", "gang"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "gang-2", "team-a", "node2", "gang"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "single", "team-a", "node2", "single"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "other", "team-b", "node3", "gang"),
		// Not scheduled yet, and not assigned to a PodGroup.
		testKAIPodEvent(workloadmeta.EventTypeSet, "pending", "team-a", "", "single"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "plain", "team-a", "node1", ""),
	})
	assertNotified(t, "node1", node1)
	assertNotified(t, "node2", node2)
	assertNotified(t, "node3", node3)
	assert.ElementsMatch(t, []string{"team-a/gang"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node1").kaiPodGroups))
	assert.ElementsMatch(t, []string{"team-a/gang", "team-a/single"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node2").kaiPodGroups))
	assert.ElementsMatch(t, []string{"team-b/gang"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node3").kaiPodGroups))

	// A changed PodGroup only wakes the nodes running its pods.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "single", "queue-c")})
	assertNotified(t, "node2", node2)
	assertNotNotified(t, "node1", node1)
	assertNotNotified(t, "node3", node3)

	// The PodGroup stays on node1 while one of its pods runs there.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeUnset, "gang-0", "team-a", "node1", "gang")})
	assertNotified(t, "node1", node1)
	assert.ElementsMatch(t, []string{"team-a/gang"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node1").kaiPodGroups))
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeUnset, "gang-1", "team-a", "node1", "gang")})
	assertNotified(t, "node1", node1)
	assert.Empty(t, srv.buildNodeMetadataState("node1").kaiPodGroups)

	// A pod scheduled later adds its PodGroup to its node.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "pending", "team-a", "node3", "single")})
	assertNotified(t, "node3", node3)
	assert.ElementsMatch(t, []string{"team-a/single", "team-b/gang"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node3").kaiPodGroups))

	// A pod that loses its annotation no longer pins its PodGroup to its node.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "single", "team-a", "node2", "")})
	assertNotified(t, "node2", node2)
	assert.ElementsMatch(t, []string{"team-a/gang"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node2").kaiPodGroups))

	// A pod update that changes neither its node nor its PodGroup wakes no one.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "gang-2", "team-a", "node2", "gang")})
	assertNotNotified(t, "node2", node2)

	// Removing every pod leaves no index entries behind.
	srv.processWmetaEvents([]workloadmeta.Event{
		testKAIPodEvent(workloadmeta.EventTypeUnset, "gang-2", "team-a", "", ""),
		testKAIPodEvent(workloadmeta.EventTypeUnset, "single", "team-a", "", ""),
		testKAIPodEvent(workloadmeta.EventTypeUnset, "other", "team-b", "", ""),
		testKAIPodEvent(workloadmeta.EventTypeUnset, "pending", "team-a", "", ""),
		testKAIPodEvent(workloadmeta.EventTypeUnset, "plain", "team-a", "", ""),
	})
	assert.Empty(t, srv.pods)
	assert.Empty(t, srv.kaiPodGroupIndex.nodeKeys)
	assert.Empty(t, srv.kaiPodGroupIndex.keyNodes)
}

func TestKAIMixedBundle(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	node1 := srv.subscribeToNamespaceEvents("node1")
	node2 := srv.subscribeToNamespaceEvents("node2")

	srv.processWmetaEvents(append(testNamespaceSetEvents(),
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "ns1", "pg", "queue-a"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "ns1", "node1", "pg"),
	))
	// The shared change wakes every node, which also covers the PodGroup change.
	assertNotified(t, "node1", node1)
	assertNotified(t, "node2", node2)
	updates, _, ok := srv.metadataUpdatesSince(0)
	require.True(t, ok)
	require.Len(t, updates, 1)
	assert.Len(t, updates[0].NamespaceMetadata, 1)
	assert.Empty(t, updates[0].KaiPodGroups)
	assert.ElementsMatch(t, []string{"ns1/pg"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node1").kaiPodGroups))

	// A pod moving to another node in one update wakes both nodes.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "ns1", "node2", "pg")})
	assertNotified(t, "node1", node1)
	assertNotified(t, "node2", node2)
	assert.Empty(t, srv.buildNodeMetadataState("node1").kaiPodGroups)
	assert.ElementsMatch(t, []string{"ns1/pg"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node2").kaiPodGroups))
}

func TestKubeMetadataStreamFilter(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "p1"}}
	podGroup := &workloadmeta.KubernetesKAIPodGroup{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKAIPodGroup, ID: "ns/pg"}}

	disabled := kubeMetadataStreamFilter(false, false)
	assert.False(t, disabled.MatchEntity(pod))
	assert.False(t, disabled.MatchEntity(podGroup))

	enabled := kubeMetadataStreamFilter(true, false)
	assert.True(t, enabled.MatchEntity(pod))
	assert.True(t, enabled.MatchEntity(podGroup))

	kueueOnly := kubeMetadataStreamFilter(false, true)
	assert.True(t, kueueOnly.MatchEntity(pod))
	assert.False(t, kueueOnly.MatchEntity(podGroup))
}

func testNamedKueueWorkloadEvent(eventType workloadmeta.EventType, namespace, name, queue string) workloadmeta.Event {
	return workloadmeta.Event{
		Type: eventType,
		Entity: &workloadmeta.KubernetesKueueWorkload{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesKueueWorkload,
				ID:   workloadmeta.GenerateKueueWorkloadEntityID(namespace, name),
			},
			EntityMeta: workloadmeta.EntityMeta{Namespace: namespace, Name: name},
			QueueName:  queue,
		},
	}
}

// testKueuePodEvent returns a pod event whose Workload comes from the Kueue
// annotation, or from the plain-Pod group label if podGroupLabel is set.
func testKueuePodEvent(eventType workloadmeta.EventType, uid, namespace, nodeName, workloadName string, podGroupLabel bool) workloadmeta.Event {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: uid},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        "pod-" + uid,
			Namespace:   namespace,
			Annotations: map[string]string{},
			Labels:      map[string]string{},
		},
		NodeName: nodeName,
	}
	if podGroupLabel {
		pod.Labels[kubernetes.KueuePodGroupNameLabelKey] = workloadName
	} else {
		pod.Annotations[kubernetes.KueueWorkloadAnnotationKey] = workloadName
	}
	return workloadmeta.Event{Type: eventType, Entity: pod}
}

func kueueWorkloadKeys(snapshot map[string]kueueWorkloadEntry) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	return keys
}

func TestKueueWorkloadsPerNode(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	srv.kueueWorkloadsPerNode = true
	srv.processWmetaEvents([]workloadmeta.Event{
		testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "job-a", "queue"),
		testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "pods-b", "queue"),
		testKueuePodEvent(workloadmeta.EventTypeSet, "a-0", "team-a", "node1", "job-a", false),
		testKueuePodEvent(workloadmeta.EventTypeSet, "a-1", "team-a", "node2", "job-a", false),
		testKueuePodEvent(workloadmeta.EventTypeSet, "b-0", "team-a", "node2", "pods-b", true),
	})

	// While the nodes running the Kueue check are unknown, every node gets
	// every Workload through the shared history.
	assert.Nil(t, srv.buildNodeMetadataState("node1").kueueWorkloads)

	srv.SetKueueCheckNodesProvider(func() (map[string]struct{}, bool) {
		return map[string]struct{}{"node3": {}}, true
	})
	assert.ElementsMatch(t, []string{"team-a/job-a"}, kueueWorkloadKeys(srv.buildNodeMetadataState("node1").kueueWorkloads))
	assert.ElementsMatch(t, []string{"team-a/job-a", "team-a/pods-b"}, kueueWorkloadKeys(srv.buildNodeMetadataState("node2").kueueWorkloads))
	assert.Nil(t, srv.buildNodeMetadataState("node3").kueueWorkloads, "the node running the Kueue check gets every Workload")
	assert.NotNil(t, srv.buildNodeMetadataState("node4").kueueWorkloads)
	assert.Empty(t, srv.buildNodeMetadataState("node4").kueueWorkloads)

	// Workloads stay in the shared history.
	updates, _, ok := srv.metadataUpdatesSince(0)
	require.True(t, ok)
	require.Len(t, updates, 1)
	assert.Len(t, updates[0].KueueWorkloads, 2)

	// A pod moving to another node in one update wakes both nodes.
	node1 := srv.subscribeToNamespaceEvents("node1")
	node4 := srv.subscribeToNamespaceEvents("node4")
	srv.processWmetaEvents([]workloadmeta.Event{testKueuePodEvent(workloadmeta.EventTypeSet, "a-0", "team-a", "node4", "job-a", false)})
	assertNotified(t, "node1", node1)
	assertNotified(t, "node4", node4)
	assert.Empty(t, srv.buildNodeMetadataState("node1").kueueWorkloads)
	assert.ElementsMatch(t, []string{"team-a/job-a"}, kueueWorkloadKeys(srv.buildNodeMetadataState("node4").kueueWorkloads))

	// A pod in both a KAI PodGroup and a Kueue Workload keeps the one it still has.
	both := testKueuePodEvent(workloadmeta.EventTypeSet, "both", "team-a", "node4", "pods-b", false)
	both.Entity.(*workloadmeta.KubernetesPod).Annotations[kubernetes.KAIPodGroupAnnotationKey] = "pg"
	srv.processWmetaEvents([]workloadmeta.Event{both, testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "pg", "queue")})
	assert.ElementsMatch(t, []string{"team-a/pg"}, kaiPodGroupKeys(srv.buildNodeMetadataState("node4").kaiPodGroups))
	srv.processWmetaEvents([]workloadmeta.Event{testKueuePodEvent(workloadmeta.EventTypeSet, "both", "team-a", "node4", "pods-b", false)})
	assert.Empty(t, srv.buildNodeMetadataState("node4").kaiPodGroups)
	assert.ElementsMatch(t, []string{"team-a/job-a", "team-a/pods-b"}, kueueWorkloadKeys(srv.buildNodeMetadataState("node4").kueueWorkloads))

	// Removing every pod leaves no index entries behind.
	srv.processWmetaEvents([]workloadmeta.Event{
		testKueuePodEvent(workloadmeta.EventTypeUnset, "a-0", "team-a", "", "", false),
		testKueuePodEvent(workloadmeta.EventTypeUnset, "a-1", "team-a", "", "", false),
		testKueuePodEvent(workloadmeta.EventTypeUnset, "b-0", "team-a", "", "", false),
		testKueuePodEvent(workloadmeta.EventTypeUnset, "both", "team-a", "", "", false),
	})
	assert.Empty(t, srv.pods)
	assert.Empty(t, srv.kueueWorkloadIndex.nodeKeys)
	assert.Empty(t, srv.kueueWorkloadIndex.keyNodes)
}

func TestRefreshKueueCheckNodes(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	srv.kueueWorkloadsPerNode = true
	node1 := srv.subscribeToNamespaceEvents("node1")
	node2 := srv.subscribeToNamespaceEvents("node2")

	checkNodes := map[string]struct{}{}
	known := false
	srv.SetKueueCheckNodesProvider(func() (map[string]struct{}, bool) {
		return checkNodes, known
	})
	assert.Nil(t, srv.kueueCheckNodes)
	assertNotNotified(t, "node1", node1)

	// Learning that no node runs the check is a change from unknown.
	known = true
	srv.refreshKueueCheckNodes()
	assert.NotNil(t, srv.kueueCheckNodes)
	assertNotified(t, "node1", node1)
	assertNotified(t, "node2", node2)

	checkNodes = map[string]struct{}{"node1": {}}
	srv.refreshKueueCheckNodes()
	assertNotified(t, "node1", node1)
	assertNotified(t, "node2", node2)

	// An unchanged set wakes no one.
	srv.refreshKueueCheckNodes()
	assertNotNotified(t, "node1", node1)

	// Losing them, for example on a leadership change, wakes every node.
	known = false
	srv.refreshKueueCheckNodes()
	assert.Nil(t, srv.kueueCheckNodes)
	assertNotified(t, "node1", node1)
	assertNotified(t, "node2", node2)

	// Without the setting, nodes always receive every Workload.
	srv.kueueWorkloadsPerNode = false
	known = true
	srv.refreshKueueCheckNodes()
	assert.Nil(t, srv.kueueCheckNodes)
}

func TestWithoutKueueWorkloads(t *testing.T) {
	workloadsOnly := &pb.KubeMetadataStreamResponse{KueueWorkloads: []*pb.KueueWorkload{{Name: "w"}}}
	mixed := &pb.KubeMetadataStreamResponse{
		IsFullState:           true,
		Mappings:              []*pb.PodServiceMapping{{PodName: "p"}},
		NamespaceMetadata:     []*pb.NamespaceMetadata{{Namespace: "ns"}},
		KueueQueues:           []*pb.KueueQueue{{Name: "q"}},
		KueueResourceFlavors:  []*pb.KueueResourceFlavor{{Name: "f"}},
		KueueWorkloads:        []*pb.KueueWorkload{{Name: "w"}},
		KaiPodGroups:          []*pb.KAIPodGroup{{Name: "pg"}},
		KubernetesAutoscalers: []*pb.KubernetesAutoscaler{{Name: "a"}},
	}
	other := &pb.KubeMetadataStreamResponse{KueueQueues: []*pb.KueueQueue{{Name: "q"}}}

	assert.Nil(t, withoutKueueWorkloads(workloadsOnly))

	expected := proto.Clone(mixed).(*pb.KubeMetadataStreamResponse)
	expected.KueueWorkloads = nil
	scoped := withoutKueueWorkloads(mixed)
	assert.True(t, proto.Equal(expected, scoped), "every field but the Workloads is kept")
	assert.Same(t, mixed.NamespaceMetadata[0], scoped.NamespaceMetadata[0], "fields are shared, not cloned")
	assert.Len(t, mixed.KueueWorkloads, 1, "shared responses are not modified")
	assert.Same(t, other, withoutKueueWorkloads(other))
}

func TestComputeKAIPodGroupsDiff(t *testing.T) {
	old := map[string]kaiPodGroupEntry{
		"ns/same":    {namespace: "ns", name: "same", queueName: "q"},
		"ns/changed": {namespace: "ns", name: "changed", queueName: "q"},
		"ns/removed": {namespace: "ns", name: "removed", queueName: "q"},
	}
	current := map[string]kaiPodGroupEntry{
		"ns/same":    {namespace: "ns", name: "same", queueName: "q"},
		"ns/changed": {namespace: "ns", name: "changed", queueName: "other"},
		"ns/added":   {namespace: "ns", name: "added", queueName: "q"},
	}

	assert.ElementsMatch(t, []*pb.KAIPodGroup{
		{Namespace: "ns", Name: "changed", Queue: "other", Type: pb.KubeMetadataEventType_SET},
		{Namespace: "ns", Name: "added", Queue: "q", Type: pb.KubeMetadataEventType_SET},
		{Namespace: "ns", Name: "removed", Queue: "q", Type: pb.KubeMetadataEventType_UNSET},
	}, computeKAIPodGroupsDiff(old, current))
	assert.Empty(t, computeKAIPodGroupsDiff(current, current))
}

func TestStreamKubeMetadata_KAIPodGroupsPerNode(t *testing.T) {
	srv := newKAIStreamTestServer()
	next := startTestStream(t, srv, "node1", nil)

	initial := next()
	assert.True(t, initial.IsFullState)
	require.Len(t, initial.KaiPodGroups, 1, "the full state only has the node's PodGroups")
	assert.Equal(t, "local", initial.KaiPodGroups[0].Name)

	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-b")})
	update := next()
	assert.False(t, update.IsFullState)
	require.Len(t, update.KaiPodGroups, 1)
	assert.Equal(t, "queue-b", update.KaiPodGroups[0].Queue)
	assert.Equal(t, pb.KubeMetadataEventType_SET, update.KaiPodGroups[0].Type)

	// Another node's PodGroup is not sent: the next response is the UNSET of
	// the pod moving to another node.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-b")})
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "team-a", "node2", "local")})
	update = next()
	require.Len(t, update.KaiPodGroups, 1)
	assert.Equal(t, "local", update.KaiPodGroups[0].Name)
	assert.Equal(t, pb.KubeMetadataEventType_UNSET, update.KaiPodGroups[0].Type)
}

func TestStreamKubeMetadata_KAIPodGroupsOnHistoryResync(t *testing.T) {
	srv := newKAIStreamTestServer()
	// The node also receives only the Kueue Workloads of its pods.
	srv.kueueWorkloadsPerNode = true
	srv.SetKueueCheckNodesProvider(func() (map[string]struct{}, bool) {
		return map[string]struct{}{}, true
	})
	srv.processWmetaEvents([]workloadmeta.Event{
		testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "local-job", "queue-a"),
		testKueuePodEvent(workloadmeta.EventTypeSet, "j1", "team-a", "node1", "local-job", false),
	})
	blocked := make(chan struct{})
	unblock := make(chan struct{})
	next := startTestStream(t, srv, "node1", func(send int) {
		if send == 2 {
			// Hold the first incremental update so the stream falls behind
			// the retained history.
			close(blocked)
			<-unblock
		}
	})

	require.True(t, next().IsFullState)

	srv.processWmetaEvents([]workloadmeta.Event{testNamespaceLabelEvent("0")})
	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the stream to send version 1")
	}
	// Overflow the history while the stream is blocked sending version 1.
	for i := 1; i <= metadataHistorySize+1; i++ {
		srv.processWmetaEvents([]workloadmeta.Event{testNamespaceLabelEvent(strconv.Itoa(i))})
	}
	// The PodGroup changes while the stream is behind.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-b")})
	close(unblock)

	assert.False(t, next().IsFullState)
	resync := next()
	require.True(t, resync.IsFullState)
	require.Len(t, resync.KaiPodGroups, 1, "the resync only has the node's PodGroups")
	assert.Equal(t, "local", resync.KaiPodGroups[0].Name)
	assert.Equal(t, "queue-b", resync.KaiPodGroups[0].Queue)
	require.Len(t, resync.KueueWorkloads, 1, "the resync only has the node's Workloads")
	assert.Equal(t, "local-job", resync.KueueWorkloads[0].Name)

	// The resync is the new baseline: the next change is incremental.
	srv.processWmetaEvents([]workloadmeta.Event{testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-c")})
	update := next()
	assert.False(t, update.IsFullState)
	require.Len(t, update.KaiPodGroups, 1)
	assert.Equal(t, "queue-c", update.KaiPodGroups[0].Queue)
}

func TestStreamKubeMetadata_KueueWorkloadsPerNode(t *testing.T) {
	srv := NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), nil)
	srv.kueueWorkloadsPerNode = true
	srv.processWmetaEvents([]workloadmeta.Event{
		testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-a"),
		testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-a"),
		testKueuePodEvent(workloadmeta.EventTypeSet, "p1", "team-a", "node1", "local", false),
		testKueuePodEvent(workloadmeta.EventTypeSet, "p2", "team-a", "node2", "remote", false),
	})
	checkNodes := map[string]struct{}{"kueue-node": {}}
	srv.SetKueueCheckNodesProvider(func() (map[string]struct{}, bool) {
		return checkNodes, true
	})
	next := startTestStream(t, srv, "node1", nil)

	initial := next()
	assert.True(t, initial.IsFullState)
	require.Len(t, initial.KueueWorkloads, 1, "the full state only has the node's Workloads")
	assert.Equal(t, "local", initial.KueueWorkloads[0].Name)

	// Another node's Workload is not sent: the next response is the node's own.
	srv.processWmetaEvents([]workloadmeta.Event{testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-b")})
	srv.processWmetaEvents([]workloadmeta.Event{testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-b")})
	update := next()
	assert.False(t, update.IsFullState)
	require.Len(t, update.KueueWorkloads, 1)
	assert.Equal(t, "local", update.KueueWorkloads[0].Name)
	assert.Equal(t, "queue-b", update.KueueWorkloads[0].Queue)

	// The Kueue check moving to the node sends it every Workload.
	checkNodes = map[string]struct{}{"node1": {}}
	srv.refreshKueueCheckNodes()
	resync := next()
	require.True(t, resync.IsFullState)
	assert.Len(t, resync.KueueWorkloads, 2)

	// It then receives every Workload change through the shared history.
	srv.processWmetaEvents([]workloadmeta.Event{testNamedKueueWorkloadEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-c")})
	update = next()
	require.Len(t, update.KueueWorkloads, 1)
	assert.Equal(t, "remote", update.KueueWorkloads[0].Name)

	// The check moving away sends it only its pods' Workloads again.
	checkNodes = map[string]struct{}{"kueue-node": {}}
	srv.refreshKueueCheckNodes()
	resync = next()
	require.True(t, resync.IsFullState)
	require.Len(t, resync.KueueWorkloads, 1)
	assert.Equal(t, "local", resync.KueueWorkloads[0].Name)
}

func testNamespaceLabelEvent(value string) workloadmeta.Event {
	return workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesMetadata{
			EntityMeta: workloadmeta.EntityMeta{
				Name:   "ns1",
				Labels: map[string]string{"version": value},
			},
		},
	}
}

// TestStartAutoscalers checks that the workloadmeta subscription includes
// autoscalers. Event processing is covered by TestProcessAutoscalerEvents.
func TestStartAutoscalers(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	srv := NewKubeMetadataStreamServer(nil, wmetaMock)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	ch := srv.subscribeToNamespaceEvents("node1")
	srv.Start(ctx)

	wmetaMock.Set(newTestAutoscaler("app-a"))
	assertNotified(t, "node1", ch)
	assert.Contains(t, srv.buildAutoscalersSnapshot(), "horizontalpodautoscaler/ns1/my-hpa")
}

func TestProcessAutoscalerEvents(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	ch := srv.subscribeToNamespaceEvents("node1")

	srv.processWmetaEvents([]workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: newTestAutoscaler("app-a")}})
	assertNotified(t, "node1", ch)
	assert.Equal(t, autoscalerEntry{
		namespace:      "ns1",
		name:           "my-hpa",
		autoscalerKind: workloadmeta.AutoscalerKindHPA,
		targetKind:     workloadmeta.AutoscalerTargetDeployment,
		targetName:     "app-a",
	}, srv.buildAutoscalersSnapshot()["horizontalpodautoscaler/ns1/my-hpa"])

	// Setting the same autoscaler again doesn't produce a diff
	srv.processWmetaEvents([]workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: newTestAutoscaler("app-a")}})
	assertNotNotified(t, "node1", ch)

	// Target change
	srv.processWmetaEvents([]workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: newTestAutoscaler("app-b")}})
	assertNotified(t, "node1", ch)
	assert.Equal(t, "app-b", srv.buildAutoscalersSnapshot()["horizontalpodautoscaler/ns1/my-hpa"].targetName)

	srv.processWmetaEvents([]workloadmeta.Event{{Type: workloadmeta.EventTypeUnset, Entity: newTestAutoscaler("app-b")}})
	assertNotified(t, "node1", ch)
	assert.Empty(t, srv.buildAutoscalersSnapshot())
}

func TestComputeAutoscalerDiff(t *testing.T) {
	old := map[string]autoscalerEntry{
		"horizontalpodautoscaler/ns1/hpa-a": {
			namespace:      "ns1",
			name:           "hpa-a",
			autoscalerKind: workloadmeta.AutoscalerKindHPA,
			targetKind:     workloadmeta.AutoscalerTargetDeployment,
			targetName:     "app-a",
		},
		"verticalpodautoscaler/ns1/vpa-a": {
			namespace:      "ns1",
			name:           "vpa-a",
			autoscalerKind: workloadmeta.AutoscalerKindVPA,
			targetKind:     workloadmeta.AutoscalerTargetStatefulSet,
			targetName:     "sts-a",
		},
		"watermarkpodautoscaler/ns1/wpa-a": {
			namespace:      "ns1",
			name:           "wpa-a",
			autoscalerKind: workloadmeta.AutoscalerKindWPA,
			targetKind:     workloadmeta.AutoscalerTargetDeployment,
			targetName:     "app-a",
		},
	}
	current := map[string]autoscalerEntry{
		"horizontalpodautoscaler/ns1/hpa-a": { // Changed
			namespace:      "ns1",
			name:           "hpa-a",
			autoscalerKind: workloadmeta.AutoscalerKindHPA,
			targetKind:     workloadmeta.AutoscalerTargetArgoRollout,
			targetName:     "rollout-a",
		},
		"watermarkpodautoscaler/ns1/wpa-a": old["watermarkpodautoscaler/ns1/wpa-a"], // Same as before
		"datadogpodautoscaler/ns1/dpa-a": { // New
			namespace:      "ns1",
			name:           "dpa-a",
			autoscalerKind: workloadmeta.AutoscalerKindDPA,
			targetKind:     workloadmeta.AutoscalerTargetDeployment,
			targetName:     "app-b",
		},
		// verticalpodautoscaler/ns1/vpa-a no longer exists
	}

	diff := computeAutoscalerDiff(old, current)

	assert.ElementsMatch(t, []*pb.KubernetesAutoscaler{
		{
			Namespace:      "ns1",
			Name:           "hpa-a",
			AutoscalerKind: "horizontalpodautoscaler",
			TargetKind:     "argo_rollout",
			TargetName:     "rollout-a",
			Type:           pb.KubeMetadataEventType_SET,
		},
		{
			Namespace:      "ns1",
			Name:           "dpa-a",
			AutoscalerKind: "datadogpodautoscaler",
			TargetKind:     "deployment",
			TargetName:     "app-b",
			Type:           pb.KubeMetadataEventType_SET,
		},
		{
			Namespace:      "ns1",
			Name:           "vpa-a",
			AutoscalerKind: "verticalpodautoscaler",
			TargetKind:     "statefulset",
			TargetName:     "sts-a",
			Type:           pb.KubeMetadataEventType_UNSET,
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
	kaiPodGroups := map[string]kaiPodGroupEntry{
		"team-a/pg": {
			namespace: "team-a",
			name:      "pg",
			queueName: "queue-a",
		},
	}
	metadata.autoscalers["horizontalpodautoscaler/ns1/my-hpa"] = autoscalerEntry{
		namespace:      "ns1",
		name:           "my-hpa",
		autoscalerKind: workloadmeta.AutoscalerKindHPA,
		targetKind:     workloadmeta.AutoscalerTargetDeployment,
		targetName:     "app-a",
	}

	resp := fullStateResponse(pods, metadata, nodeMetadataState{kaiPodGroups: kaiPodGroups})

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
		KaiPodGroups: []*pb.KAIPodGroup{
			{
				Namespace: "team-a",
				Name:      "pg",
				Queue:     "queue-a",
				Type:      pb.KubeMetadataEventType_SET,
			},
		},
		KubernetesAutoscalers: []*pb.KubernetesAutoscaler{
			{
				Namespace:      "ns1",
				Name:           "my-hpa",
				AutoscalerKind: "horizontalpodautoscaler",
				TargetKind:     "deployment",
				TargetName:     "app-a",
				Type:           pb.KubeMetadataEventType_SET,
			},
		},
	}

	assert.True(t, proto.Equal(expected, resp))
}

func TestMetadataUpdatesSince(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	ch := srv.subscribeToNamespaceEvents("node1")
	events := append(testNamespaceSetEvents(), testKueueWorkloadEvent("first", workloadmeta.EventTypeSet))
	srv.processWmetaEvents(events)
	assertNotified(t, "node1", ch)

	initial, version := srv.buildMetadataSnapshotWithVersion()
	assert.Equal(t, uint64(1), version)
	assert.Equal(t, "first", initial.kueueWorkloads["ns1/job"].uid)
	updates, version, ok := srv.metadataUpdatesSince(0)
	require.True(t, ok)
	require.Len(t, updates, 1)
	assert.Equal(t, uint64(1), version)
	assert.False(t, updates[0].IsFullState)
	assert.Len(t, updates[0].NamespaceMetadata, 1)
	require.Len(t, updates[0].KueueWorkloads, 1)
	first := updates[0]

	// Identical state neither advances the journal nor wakes subscribers.
	srv.processWmetaEvents(events)
	updates, version, ok = srv.metadataUpdatesSince(version)
	require.True(t, ok)
	assert.Empty(t, updates)
	assert.Equal(t, uint64(1), version)
	assertNotNotified(t, "node1", ch)

	srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("second", workloadmeta.EventTypeSet)})
	fastUpdates, fastVersion, ok := srv.metadataUpdatesSince(version)
	require.True(t, ok)
	require.Len(t, fastUpdates, 1)
	assert.Equal(t, uint64(2), fastVersion)

	// A slow client coalesces notifications, but must still see the deletion
	// followed by recreation. A fast client's cursor is independent.
	srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("second", workloadmeta.EventTypeUnset)})
	srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("third", workloadmeta.EventTypeSet)})
	slowUpdates, slowVersion, ok := srv.metadataUpdatesSince(version)
	require.True(t, ok)
	require.Len(t, slowUpdates, 3)
	assert.Equal(t, uint64(4), slowVersion)
	assert.Same(t, fastUpdates[0], slowUpdates[0], "clients share the same precomputed response")
	assert.Equal(t, "second", slowUpdates[0].KueueWorkloads[0].Uid)
	assert.Equal(t, pb.KubeMetadataEventType_UNSET, slowUpdates[1].KueueWorkloads[0].Type)
	assert.Equal(t, "third", slowUpdates[2].KueueWorkloads[0].Uid)
	assert.Equal(t, "first", first.KueueWorkloads[0].Uid, "published responses remain immutable")

	fastUpdates, fastVersion, ok = srv.metadataUpdatesSince(fastVersion)
	require.True(t, ok)
	require.Len(t, fastUpdates, 2)
	assert.Equal(t, slowVersion, fastVersion)
	assert.Same(t, slowUpdates[1], fastUpdates[0])
	assert.Same(t, slowUpdates[2], fastUpdates[1])
}

func TestMetadataUpdatesSinceHistoryOverflow(t *testing.T) {
	srv := NewKubeMetadataStreamServer(nil, nil)
	for i := 1; i <= metadataHistorySize; i++ {
		srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent(strconv.Itoa(i), workloadmeta.EventTypeSet)})
	}
	updates, version, ok := srv.metadataUpdatesSince(0)
	require.True(t, ok, "the oldest retained cursor is still valid")
	require.Len(t, updates, metadataHistorySize)
	assert.Equal(t, uint64(metadataHistorySize), version)
	first := updates[0]

	srv.processWmetaEvents([]workloadmeta.Event{testKueueWorkloadEvent("latest", workloadmeta.EventTypeSet)})
	_, _, ok = srv.metadataUpdatesSince(0)
	assert.False(t, ok, "an overwritten cursor must request a full state")
	updates, version, ok = srv.metadataUpdatesSince(1)
	require.True(t, ok)
	require.Len(t, updates, metadataHistorySize)
	assert.Equal(t, uint64(metadataHistorySize+1), version)
	assert.Equal(t, "2", updates[0].KueueWorkloads[0].Uid)
	assert.Equal(t, "latest", updates[len(updates)-1].KueueWorkloads[0].Uid)
	assert.Equal(t, "1", first.KueueWorkloads[0].Uid, "overwriting history cannot mutate in-flight responses")
}

func testKueueWorkloadEvent(uid string, eventType workloadmeta.EventType) workloadmeta.Event {
	return workloadmeta.Event{
		Type: eventType,
		Entity: &workloadmeta.KubernetesKueueWorkload{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueWorkload, ID: "ns1/job"},
			EntityMeta: workloadmeta.EntityMeta{
				Namespace: "ns1",
				Name:      "job",
				UID:       uid,
				Labels:    map[string]string{"team": "example"},
			},
		},
	}
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

func newTestAutoscaler(targetName string) *workloadmeta.KubernetesAutoscaler {
	return &workloadmeta.KubernetesAutoscaler{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesAutoscaler,
			ID:   "horizontalpodautoscaler/ns1/my-hpa",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "my-hpa",
			Namespace: "ns1",
		},
		AutoscalerKind: workloadmeta.AutoscalerKindHPA,
		TargetKind:     workloadmeta.AutoscalerTargetDeployment,
		TargetName:     targetName,
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

// mockStream implements pb.AgentSecure_StreamKubeMetadataServer for testing.
type mockStream struct {
	ctx      context.Context
	sent     []*pb.KubeMetadataStreamResponse
	sendErr  error
	sendFunc func(*pb.KubeMetadataStreamResponse) error // optional per-call control
}

func (m *mockStream) Send(resp *pb.KubeMetadataStreamResponse) error {
	if m.sendFunc != nil {
		return m.sendFunc(resp)
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, resp)
	return nil
}

func (m *mockStream) Context() context.Context     { return m.ctx }
func (m *mockStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockStream) SendHeader(metadata.MD) error { return nil }
func (m *mockStream) SetTrailer(metadata.MD)       {}
func (m *mockStream) SendMsg(interface{}) error    { return nil }
func (m *mockStream) RecvMsg(interface{}) error    { return nil }

// newKAIStreamTestServer returns a server with the "local" PodGroup on node1
// and the "remote" PodGroup on node2.
func newKAIStreamTestServer() *KubeMetadataStreamServer {
	srv := NewKubeMetadataStreamServer(controllers.GetGlobalMetaBundleStore(), nil)
	srv.processWmetaEvents([]workloadmeta.Event{
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "local", "queue-a"),
		testKAIPodGroupEvent(workloadmeta.EventTypeSet, "team-a", "remote", "queue-a"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "p1", "team-a", "node1", "local"),
		testKAIPodEvent(workloadmeta.EventTypeSet, "p2", "team-a", "node2", "remote"),
	})
	return srv
}

// startTestStream streams to nodeName until the test ends and returns a
// function that waits for the next response. beforeSend, if set, is called
// with the number of the send before each one.
func startTestStream(t *testing.T, srv *KubeMetadataStreamServer, nodeName string, beforeSend func(send int)) func() *pb.KubeMetadataStreamResponse {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	sent := make(chan *pb.KubeMetadataStreamResponse, 16)
	sends := 0
	stream := &mockStream{
		ctx: ctx,
		sendFunc: func(resp *pb.KubeMetadataStreamResponse) error {
			sends++
			if beforeSend != nil {
				beforeSend(sends)
			}
			sent <- resp
			return nil
		},
	}
	done := make(chan error)
	go func() {
		done <- srv.StreamKubeMetadata(&pb.KubeMetadataStreamRequest{NodeName: nodeName}, stream)
	}()
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})

	return func() *pb.KubeMetadataStreamResponse {
		t.Helper()
		select {
		case resp := <-sent:
			return resp
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for a stream response")
			return nil
		}
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && kubelet

package kubemetadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

type expectedPod struct {
	services      []string
	nsLabels      map[string]string
	nsAnnotations map[string]string
}

type expectedKueueQueue struct {
	namespace        string
	name             string
	clusterQueueName string
	labels           map[string]string
	annotations      map[string]string
	uid              string
}

type expectedKueueResourceFlavor struct {
	name               string
	nodeAffinityLabels map[string]string
	labels             map[string]string
	annotations        map[string]string
	uid                string
}

type expectedKueueWorkload struct {
	namespace         string
	name              string
	queueName         string
	clusterQueueName  string
	labels            map[string]string
	annotations       map[string]string
	uid               string
	podSetAssignments []workloadmeta.KueuePodSetAssignment
}

// This is a simple test for run(). Exhaustive tests for the individual
// functions it calls are defined below.
func TestStreamingProvider_run(t *testing.T) {
	podName := "pod1"
	podUID := "uid-1"
	namespace := "default"

	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	stream := newDCAStreamClient("node-a", nil)
	provider := &streamingProvider{
		dcaStream:              stream,
		wmeta:                  wmetaMock,
		collectNamespaceLabels: true,
	}

	// DCA returns initial state with the test pod
	stream.applyResponse(&pb.KubeMetadataStreamResponse{
		IsFullState: true,
		Mappings: []*pb.PodServiceMapping{
			{
				Namespace:    namespace,
				PodName:      podName,
				ServiceNames: []string{"svc-a"},
				Type:         pb.KubeMetadataEventType_SET,
			},
		},
		NamespaceMetadata: []*pb.NamespaceMetadata{
			{
				Namespace: namespace,
				Labels:    map[string]string{"l1": "v1"},
				Type:      pb.KubeMetadataEventType_SET,
			},
		},
	})

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go provider.run(ctx)

	// Kubelet discovers the test pod
	wmetaMock.Notify([]workloadmeta.CollectorEvent{{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceNodeOrchestrator,
		Entity: &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   podUID,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      podName,
				Namespace: namespace,
			},
			Ready: true,
		},
	}})

	// The service reported by the DCA should be available
	assert.Eventually(t, func() bool {
		pod, err := wmetaMock.GetKubernetesPod(podUID)
		return err == nil && len(pod.KubeServices) > 0
	}, 5*time.Second, 10*time.Millisecond)
	pod, _ := wmetaMock.GetKubernetesPod(podUID)
	assert.Equal(t, []string{"svc-a"}, pod.KubeServices)
	assert.Equal(t, map[string]string{"l1": "v1"}, pod.NamespaceLabels)

	// DCA adds a new service for the test pod
	stream.applyResponse(&pb.KubeMetadataStreamResponse{
		Mappings: []*pb.PodServiceMapping{
			{
				Namespace:    namespace,
				PodName:      podName,
				ServiceNames: []string{"svc-a", "svc-b"},
				Type:         pb.KubeMetadataEventType_SET,
			},
		},
	})

	assert.Eventually(t, func() bool {
		pod, err := wmetaMock.GetKubernetesPod(podUID)
		return err == nil && len(pod.KubeServices) == 2
	}, 5*time.Second, 10*time.Millisecond)
	pod, _ = wmetaMock.GetKubernetesPod(podUID)
	assert.ElementsMatch(t, []string{"svc-a", "svc-b"}, pod.KubeServices)
	assert.Equal(t, map[string]string{"l1": "v1"}, pod.NamespaceLabels) // Unchanged
}

func TestStreamingProvider_handleWmetaPodEvents(t *testing.T) {
	tests := []struct {
		name string

		// Streaming provider config
		collectNamespaceLabels      bool
		collectNamespaceAnnotations bool
		ignoreServiceReadiness      bool

		// DCA stream state
		dcaResponse *pb.KubeMetadataStreamResponse

		// Pre-existing state
		preExistingEvents []workloadmeta.CollectorEvent
		initialSeenPods   map[string]string

		// Input
		bundleEvents []workloadmeta.Event

		// Assertions
		expectedPods     map[string]expectedPod
		expectedSeenPods map[string]string
	}{
		{
			name:                        "set enriches pod with services and namespace metadata",
			collectNamespaceLabels:      true,
			collectNamespaceAnnotations: true,
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a", "svc-b"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace:   "default",
						Labels:      map[string]string{"env": "prod"},
						Annotations: map[string]string{"a1": "v1"},
						Type:        pb.KubeMetadataEventType_SET,
					},
				},
			},
			bundleEvents: []workloadmeta.Event{
				makePodSetEvent("uid-1", "default", "pod1", true),
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {
					services:      []string{"svc-a", "svc-b"},
					nsLabels:      map[string]string{"env": "prod"},
					nsAnnotations: map[string]string{"a1": "v1"},
				},
			},
			expectedSeenPods: map[string]string{"default/pod1": "uid-1"},
		},
		{
			name: "unset removes pod",
			preExistingEvents: []workloadmeta.CollectorEvent{{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceClusterOrchestrator,
				Entity: &workloadmeta.KubernetesPod{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "uid-1",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod1",
						Namespace: "default",
					},
				},
			}},
			initialSeenPods: map[string]string{"default/pod1": "uid-1"},
			bundleEvents: []workloadmeta.Event{
				makePodUnsetEvent("uid-1"),
			},
			expectedSeenPods: map[string]string{},
		},
		{
			name:                        "mixed bundle with 2 sets and 1 unset",
			collectNamespaceLabels:      true,
			collectNamespaceAnnotations: true,
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
					{
						Namespace:    "default",
						PodName:      "pod2",
						ServiceNames: []string{"svc-b"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace: "default",
						Labels:    map[string]string{"env": "prod"},
						Type:      pb.KubeMetadataEventType_SET,
					},
				},
			},
			preExistingEvents: []workloadmeta.CollectorEvent{{
				Type:   workloadmeta.EventTypeSet,
				Source: workloadmeta.SourceClusterOrchestrator,
				Entity: &workloadmeta.KubernetesPod{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindKubernetesPod,
						ID:   "uid-3",
					},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "pod3",
						Namespace: "default",
					},
				},
			}},
			initialSeenPods: map[string]string{"default/pod3": "uid-3"},
			bundleEvents: []workloadmeta.Event{
				makePodSetEvent("uid-1", "default", "pod1", true),
				makePodSetEvent("uid-2", "default", "pod2", true),
				makePodUnsetEvent("uid-3"),
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {services: []string{"svc-a"}, nsLabels: map[string]string{"env": "prod"}},
				"uid-2": {services: []string{"svc-b"}, nsLabels: map[string]string{"env": "prod"}},
			},
			expectedSeenPods: map[string]string{"default/pod1": "uid-1", "default/pod2": "uid-2"},
		},
		{
			name:                   "not-ready pod gets empty services when ignoreServiceReadiness=false",
			ignoreServiceReadiness: false,
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
			},
			bundleEvents: []workloadmeta.Event{
				makePodSetEvent("uid-1", "default", "pod1", false),
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {services: []string{}},
			},
		},
		{
			name:                   "not-ready pod gets services when ignoreServiceReadiness=true",
			ignoreServiceReadiness: true,
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
			},
			bundleEvents: []workloadmeta.Event{
				makePodSetEvent("uid-1", "default", "pod1", false),
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {services: []string{"svc-a"}},
			},
		},
		{
			name:                        "namespace annotations excluded when collectNamespaceAnnotations=false",
			collectNamespaceLabels:      true,
			collectNamespaceAnnotations: false,
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace:   "default",
						Labels:      map[string]string{"env": "prod"},
						Annotations: map[string]string{"note": "important"},
						Type:        pb.KubeMetadataEventType_SET,
					},
				},
			},
			bundleEvents: []workloadmeta.Event{
				makePodSetEvent("uid-1", "default", "pod1", true),
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {
					services: []string{"svc-a"},
					nsLabels: map[string]string{"env": "prod"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Initialize provider
			wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))
			provider := &streamingProvider{
				dcaStream:                   newDCAStreamClient("node-a", nil),
				wmeta:                       wmetaMock,
				collectNamespaceLabels:      test.collectNamespaceLabels,
				collectNamespaceAnnotations: test.collectNamespaceAnnotations,
				ignoreServiceReadiness:      test.ignoreServiceReadiness,
			}

			// Apply initial state
			if test.dcaResponse != nil {
				provider.dcaStream.applyResponse(test.dcaResponse)
			}
			if len(test.preExistingEvents) > 0 {
				wmetaMock.Notify(test.preExistingEvents)
			}
			seenPods := make(map[string]string)
			for namespacedName, podUID := range test.initialSeenPods {
				seenPods[namespacedName] = podUID
			}

			// Test function
			provider.handleWmetaPodEvents(makePodBundle(test.bundleEvents...), seenPods)

			// Asserts
			assert.Len(t, wmetaMock.ListKubernetesPods(), len(test.expectedPods))
			for podUID, expected := range test.expectedPods {
				pod, err := wmetaMock.GetKubernetesPod(podUID)
				require.NoError(t, err)
				assert.ElementsMatch(t, expected.services, pod.KubeServices)
				assert.Equal(t, expected.nsLabels, pod.NamespaceLabels)
				assert.Equal(t, expected.nsAnnotations, pod.NamespaceAnnotations)
			}
			if test.expectedSeenPods != nil {
				assert.Equal(t, test.expectedSeenPods, seenPods)
			}
		})
	}
}

func TestStreamingProvider_handleDCAStreamUpdate(t *testing.T) {
	tests := []struct {
		name string

		// Pre-existing state
		dcaResponse       *pb.KubeMetadataStreamResponse
		preExistingEvents []workloadmeta.CollectorEvent
		initialSeenPods   map[string]string

		update                       streamUpdate
		expectedPods                 map[string]expectedPod
		expectedKueueQueues          map[string]expectedKueueQueue
		expectedKueueResourceFlavors map[string]expectedKueueResourceFlavor
		expectedKueueWorkloads       map[string]expectedKueueWorkload
	}{
		{
			name: "full state emits Kueue queue entities and re-enriches all seen pods",
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
					{
						Namespace:    "default",
						PodName:      "pod2",
						ServiceNames: []string{"svc-b"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace:   "default",
						Labels:      map[string]string{"l1": "v1"},
						Annotations: map[string]string{"a1": "v1"},
						Type:        pb.KubeMetadataEventType_SET,
					},
				},
				KueueQueues: []*pb.KueueQueue{
					{
						Namespace:    "default",
						Name:         "batch",
						QueueType:    pb.KueueQueueType_LOCAL_QUEUE,
						ClusterQueue: "cluster-batch",
						Labels:       map[string]string{"queue": "batch"},
						Annotations:  map[string]string{"owner": "team-a"},
						Uid:          "queue-uid",
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				KueueResourceFlavors: []*pb.KueueResourceFlavor{
					{
						Name:               "a100",
						Labels:             map[string]string{"flavor": "gpu"},
						Annotations:        map[string]string{"owner": "team-a"},
						Uid:                "flavor-uid",
						NodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
						Type:               pb.KubeMetadataEventType_SET,
					},
				},
				KueueWorkloads: []*pb.KueueWorkload{
					{
						Namespace:    "default",
						Name:         "job-sample",
						Queue:        "batch",
						ClusterQueue: "cluster-batch",
						Labels:       map[string]string{"workload": "sample"},
						Annotations:  map[string]string{"owner": "team-a"},
						Uid:          "workload-uid",
						PodSetAssignments: []*pb.KueuePodSetAssignment{
							{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
						},
						Type: pb.KubeMetadataEventType_SET,
					},
				},
			},
			preExistingEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceNodeOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "uid-1",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "pod1",
							Namespace: "default",
						},
						Ready: true,
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceNodeOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "uid-2",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "pod2",
							Namespace: "default",
						},
						Ready: true,
					},
				},
			},
			initialSeenPods: map[string]string{"default/pod1": "uid-1", "default/pod2": "uid-2"},
			update: streamUpdate{
				updateIsFullState: true,
				updatedKueueQueues: map[string]struct{}{
					"localqueue/default/batch": {},
				},
				updatedKueueResourceFlavors: map[string]struct{}{
					"a100": {},
				},
				updatedKueueWorkloads: map[string]struct{}{
					"default/job-sample": {},
				},
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {
					services:      []string{"svc-a"},
					nsLabels:      map[string]string{"l1": "v1"},
					nsAnnotations: map[string]string{"a1": "v1"},
				},
				"uid-2": {
					services:      []string{"svc-b"},
					nsLabels:      map[string]string{"l1": "v1"},
					nsAnnotations: map[string]string{"a1": "v1"},
				},
			},
			expectedKueueQueues: map[string]expectedKueueQueue{
				"localqueue/default/batch": {
					namespace:        "default",
					name:             "batch",
					clusterQueueName: "cluster-batch",
					labels:           map[string]string{"queue": "batch"},
					annotations:      map[string]string{"owner": "team-a"},
					uid:              "queue-uid",
				},
			},
			expectedKueueResourceFlavors: map[string]expectedKueueResourceFlavor{
				"a100": {
					name:               "a100",
					labels:             map[string]string{"flavor": "gpu"},
					annotations:        map[string]string{"owner": "team-a"},
					uid:                "flavor-uid",
					nodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
				},
			},
			expectedKueueWorkloads: map[string]expectedKueueWorkload{
				"default/job-sample": {
					namespace:        "default",
					name:             "job-sample",
					queueName:        "batch",
					clusterQueueName: "cluster-batch",
					labels:           map[string]string{"workload": "sample"},
					annotations:      map[string]string{"owner": "team-a"},
					uid:              "workload-uid",
					podSetAssignments: []workloadmeta.KueuePodSetAssignment{
						{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
					},
				},
			},
		},
		{
			name: "incremental update enriches only updated pods and namespaces",
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
					{
						Namespace:    "default",
						PodName:      "pod2",
						ServiceNames: []string{"svc-b"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace:   "default",
						Labels:      map[string]string{"l1": "v1"},
						Annotations: map[string]string{"a1": "v1"},
						Type:        pb.KubeMetadataEventType_SET,
					},
				},
			},
			preExistingEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceNodeOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "uid-1",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "pod1",
							Namespace: "default",
						},
						Ready: true,
					},
				},
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceNodeOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "uid-2",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "pod2",
							Namespace: "default",
						},
						Ready: true,
					},
				},
			},
			initialSeenPods: map[string]string{"default/pod1": "uid-1", "default/pod2": "uid-2"},
			update: streamUpdate{
				// pod2 is not in the update
				updatedPods:       map[string]struct{}{"default/pod1": {}},
				updatedNamespaces: map[string]struct{}{"default": {}},
			},
			expectedPods: map[string]expectedPod{
				// Only uid-1 is updated, because uid-2 is not in the update
				"uid-1": {
					services:      []string{"svc-a"},
					nsLabels:      map[string]string{"l1": "v1"},
					nsAnnotations: map[string]string{"a1": "v1"},
				},
			},
		},
		{
			name: "namespace-only update re-enriches pods in that namespace",
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
				NamespaceMetadata: []*pb.NamespaceMetadata{
					{
						Namespace:   "default",
						Labels:      map[string]string{"l1": "v1"},
						Annotations: map[string]string{"a1": "v1"},
						Type:        pb.KubeMetadataEventType_SET,
					},
				},
			},
			preExistingEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceNodeOrchestrator,
					Entity: &workloadmeta.KubernetesPod{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesPod,
							ID:   "uid-1",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "pod1",
							Namespace: "default",
						},
						Ready: true,
					},
				},
			},
			initialSeenPods: map[string]string{"default/pod1": "uid-1"},
			update: streamUpdate{
				// No updated pods, only namespace metadata changed
				updatedNamespaces: map[string]struct{}{"default": {}},
			},
			expectedPods: map[string]expectedPod{
				"uid-1": {
					services:      []string{"svc-a"},
					nsLabels:      map[string]string{"l1": "v1"},
					nsAnnotations: map[string]string{"a1": "v1"},
				},
			},
		},
		{
			name: "update skips pod in seenPods but missing from wmeta",
			dcaResponse: &pb.KubeMetadataStreamResponse{
				IsFullState: true,
				Mappings: []*pb.PodServiceMapping{
					{
						Namespace:    "default",
						PodName:      "pod1",
						ServiceNames: []string{"svc-a"},
						Type:         pb.KubeMetadataEventType_SET,
					},
				},
			},
			// pod1 is in seenPods but not in wmeta
			initialSeenPods: map[string]string{"default/pod1": "uid-1"},
			update: streamUpdate{
				updatedPods: map[string]struct{}{"default/pod1": {}},
			},
			expectedPods: map[string]expectedPod{},
		},
		{
			name: "Kueue queue unset removes local entity",
			preExistingEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesKueueQueue{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesKueueQueue,
							ID:   "localqueue/default/batch",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "batch",
							Namespace: "default",
						},
						QueueType:        workloadmeta.KueueLocalQueue,
						ClusterQueueName: "cluster-batch",
					},
				},
			},
			update: streamUpdate{
				updatedKueueQueues: map[string]struct{}{
					"localqueue/default/batch": {},
				},
			},
			expectedPods:        map[string]expectedPod{},
			expectedKueueQueues: map[string]expectedKueueQueue{},
		},
		{
			name: "Kueue ResourceFlavor unset removes local entity",
			preExistingEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesKueueResourceFlavor{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesKueueResourceFlavor,
							ID:   "a100",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name: "a100",
						},
						NodeAffinityLabels: map[string]string{"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB"},
					},
				},
			},
			update: streamUpdate{
				updatedKueueResourceFlavors: map[string]struct{}{
					"a100": {},
				},
			},
			expectedPods:                 map[string]expectedPod{},
			expectedKueueResourceFlavors: map[string]expectedKueueResourceFlavor{},
		},
		{
			name: "Kueue Workload unset removes local entity",
			preExistingEvents: []workloadmeta.CollectorEvent{
				{
					Type:   workloadmeta.EventTypeSet,
					Source: workloadmeta.SourceClusterOrchestrator,
					Entity: &workloadmeta.KubernetesKueueWorkload{
						EntityID: workloadmeta.EntityID{
							Kind: workloadmeta.KindKubernetesKueueWorkload,
							ID:   "default/job-sample",
						},
						EntityMeta: workloadmeta.EntityMeta{
							Name:      "job-sample",
							Namespace: "default",
						},
						QueueName:        "batch",
						ClusterQueueName: "cluster-batch",
						PodSetAssignments: []workloadmeta.KueuePodSetAssignment{
							{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
						},
					},
				},
			},
			update: streamUpdate{
				updatedKueueWorkloads: map[string]struct{}{
					"default/job-sample": {},
				},
			},
			expectedPods:           map[string]expectedPod{},
			expectedKueueWorkloads: map[string]expectedKueueWorkload{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Initialize provider
			wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
				core.MockBundle(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
			))
			provider := &streamingProvider{
				dcaStream:                   newDCAStreamClient("node-a", nil),
				wmeta:                       wmetaMock,
				collectNamespaceLabels:      true,
				collectNamespaceAnnotations: true,
			}

			// Apply initial state
			if test.dcaResponse != nil {
				provider.dcaStream.applyResponse(test.dcaResponse)
				provider.dcaStream.drainPendingUpdate()
			}
			if len(test.preExistingEvents) > 0 {
				wmetaMock.Notify(test.preExistingEvents)
			}
			seenPods := make(map[string]string)
			for namespacedName, podUID := range test.initialSeenPods {
				seenPods[namespacedName] = podUID
			}

			// Test function
			provider.handleDCAStreamUpdate(test.update, seenPods)

			// Asserts
			for podUID, expected := range test.expectedPods {
				pod, err := wmetaMock.GetKubernetesPod(podUID)
				require.NoError(t, err)
				assert.ElementsMatch(t, expected.services, pod.KubeServices)
				assert.Equal(t, expected.nsLabels, pod.NamespaceLabels)
				assert.Equal(t, expected.nsAnnotations, pod.NamespaceAnnotations)
			}
			assertKueueQueues(t, wmetaMock, test.expectedKueueQueues)
			assertKueueResourceFlavors(t, wmetaMock, test.expectedKueueResourceFlavors)
			assertKueueWorkloads(t, wmetaMock, test.expectedKueueWorkloads)
		})
	}
}

func TestStreamingProvider_handleDCAStreamUpdate_FlavorReenrichesJoinedPod(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	provider := &streamingProvider{
		dcaStream:                   newDCAStreamClient("node-a", nil),
		wmeta:                       wmetaMock,
		collectNamespaceLabels:      true,
		collectNamespaceAnnotations: true,
	}

	// Seed the DCA stream: a workload referencing flavor "a100", plus service
	// mappings for both pods. Re-enrichment is observable because a re-enriched
	// pod picks up its KubeServices from this cache.
	provider.dcaStream.applyResponse(&pb.KubeMetadataStreamResponse{
		IsFullState: true,
		Mappings: []*pb.PodServiceMapping{
			{Namespace: "default", PodName: "pod-joined", ServiceNames: []string{"svc-joined"}, Type: pb.KubeMetadataEventType_SET},
			{Namespace: "default", PodName: "pod-unrelated", ServiceNames: []string{"svc-unrelated"}, Type: pb.KubeMetadataEventType_SET},
		},
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace: "default",
				Name:      "job-sample",
				PodSetAssignments: []*pb.KueuePodSetAssignment{
					{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
				},
				Type: pb.KubeMetadataEventType_SET,
			},
		},
	})
	provider.dcaStream.drainPendingUpdate()

	// pod-joined joins the workload via the Kueue workload annotation; pod-unrelated does not.
	wmetaMock.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-joined"},
				EntityMeta: workloadmeta.EntityMeta{Name: "pod-joined", Namespace: "default", Annotations: map[string]string{kubernetes.KueueWorkloadAnnotationKey: "job-sample"}},
				Ready:      true,
			},
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourceNodeOrchestrator,
			Entity: &workloadmeta.KubernetesPod{
				EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-unrelated"},
				EntityMeta: workloadmeta.EntityMeta{Name: "pod-unrelated", Namespace: "default"},
				Ready:      true,
			},
		},
	})

	seenPods := map[string]string{"default/pod-joined": "uid-joined", "default/pod-unrelated": "uid-unrelated"}

	// Only a ResourceFlavor changed. The joined pod must be re-enriched
	// (transitively through its workload); the unrelated pod must not.
	provider.handleDCAStreamUpdate(streamUpdate{
		updatedKueueResourceFlavors: map[string]struct{}{"a100": {}},
	}, seenPods)

	joined, err := wmetaMock.GetKubernetesPod("uid-joined")
	require.NoError(t, err)
	assert.Equal(t, []string{"svc-joined"}, joined.KubeServices)

	unrelated, err := wmetaMock.GetKubernetesPod("uid-unrelated")
	require.NoError(t, err)
	assert.Empty(t, unrelated.KubeServices)
}

func TestStreamingProvider_podAffectedByKueueUpdate(t *testing.T) {
	provider := &streamingProvider{
		dcaStream: newDCAStreamClient("node-a", nil),
	}
	// Seed the DCA stream cache with a Workload that references flavor "a100"
	// so transitive ResourceFlavor updates can be resolved.
	provider.dcaStream.applyResponse(&pb.KubeMetadataStreamResponse{
		IsFullState: true,
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace: "default",
				Name:      "job-sample",
				PodSetAssignments: []*pb.KueuePodSetAssignment{
					{Name: "main", Flavors: map[string]string{"nvidia.com/gpu": "a100"}},
				},
				Type: pb.KubeMetadataEventType_SET,
			},
		},
	})
	provider.dcaStream.drainPendingUpdate()

	podWithWorkloadAnnotation := &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{
			Namespace:   "default",
			Annotations: map[string]string{kubernetes.KueueWorkloadAnnotationKey: "job-sample"},
		},
	}
	podWithPodGroupLabel := &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{
			Namespace: "default",
			Labels:    map[string]string{kubernetes.KueuePodGroupNameLabelKey: "job-sample"},
		},
	}
	podWithoutKueue := &workloadmeta.KubernetesPod{
		EntityMeta: workloadmeta.EntityMeta{Namespace: "default"},
	}

	tests := []struct {
		name     string
		pod      *workloadmeta.KubernetesPod
		update   streamUpdate
		expected bool
	}{
		{
			name:     "pod not managed by Kueue is never affected",
			pod:      podWithoutKueue,
			update:   streamUpdate{updatedKueueWorkloads: map[string]struct{}{"default/job-sample": {}}},
			expected: false,
		},
		{
			name:     "pod joins updated Workload via annotation",
			pod:      podWithWorkloadAnnotation,
			update:   streamUpdate{updatedKueueWorkloads: map[string]struct{}{"default/job-sample": {}}},
			expected: true,
		},
		{
			name:     "pod joins updated Workload via pod-group label",
			pod:      podWithPodGroupLabel,
			update:   streamUpdate{updatedKueueWorkloads: map[string]struct{}{"default/job-sample": {}}},
			expected: true,
		},
		{
			name:     "pod joins a different Workload than the updated one",
			pod:      podWithWorkloadAnnotation,
			update:   streamUpdate{updatedKueueWorkloads: map[string]struct{}{"default/other": {}}},
			expected: false,
		},
		{
			name:     "pod affected transitively by updated ResourceFlavor",
			pod:      podWithWorkloadAnnotation,
			update:   streamUpdate{updatedKueueResourceFlavors: map[string]struct{}{"a100": {}}},
			expected: true,
		},
		{
			name:     "pod not affected by unrelated ResourceFlavor update",
			pod:      podWithWorkloadAnnotation,
			update:   streamUpdate{updatedKueueResourceFlavors: map[string]struct{}{"h100": {}}},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, provider.podAffectedByKueueUpdate(test.pod, test.update))
		})
	}
}

func TestStreamingProvider_isActive(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	provider := &streamingProvider{
		dcaStream: newDCAStreamClient("node-a", nil),
		wmeta:     wmetaMock,
	}

	require.False(t, provider.isActive())

	ctx, cancel := context.WithCancel(context.TODO())
	provider.start(ctx)
	require.True(t, provider.isActive())

	cancel()
	assert.Eventually(t, func() bool {
		return !provider.isActive()
	}, 5*time.Second, 10*time.Millisecond)
}

func TestStreamingProvider_isActive_unimplemented(t *testing.T) {
	wmetaMock := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	stream := newDCAStreamClient("node-a", nil)

	// Simulate the DCA returning unimplemented
	stream.mu.Lock()
	stream.unimplemented = true
	stream.mu.Unlock()
	stream.signalReady()

	provider := &streamingProvider{
		dcaStream: stream,
		wmeta:     wmetaMock,
	}

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	provider.start(ctx)

	assert.Eventually(t, func() bool {
		return !provider.isActive()
	}, 5*time.Second, 10*time.Millisecond)
}

func TestDCAStreamClient_ApplyResponse(t *testing.T) {
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
			sc := &dcaStreamClient{
				podServices: test.initialPodServices,
				namespaces:  test.initialNamespaces,
				kueueQueues: make(map[string]*workloadmeta.KubernetesKueueQueue),
				initialized: test.initialActive,
				readyCh:     make(chan struct{}),
				updateCh:    make(chan struct{}, 1),
			}

			sc.applyResponse(test.response)

			assert.True(t, sc.initialized)
			assert.Equal(t, test.expectedPodServices, sc.podServices)
			assert.Equal(t, test.expectedNamespaces, sc.namespaces)
		})
	}
}

func TestDCAStreamClient_ApplyResponse_KueueWorkloads(t *testing.T) {
	sc := newDCAStreamClient("node-a", nil)

	// Full state seeds the workload cache.
	sc.applyResponse(&pb.KubeMetadataStreamResponse{
		IsFullState: true,
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace:    "default",
				Name:         "job-a",
				Queue:        "batch",
				ClusterQueue: "cluster-batch",
				Type:         pb.KubeMetadataEventType_SET,
			},
		},
	})
	update := sc.drainPendingUpdate()
	assert.True(t, update.updateIsFullState)
	assert.Contains(t, update.updatedKueueWorkloads, "default/job-a")
	require.Contains(t, sc.kueueWorkloads, "default/job-a")
	assert.Equal(t, "batch", sc.kueueWorkloads["default/job-a"].QueueName)

	// Incremental SET of a new workload adds to the cache.
	sc.applyResponse(&pb.KubeMetadataStreamResponse{
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace: "default",
				Name:      "job-b",
				Queue:     "gpu",
				Type:      pb.KubeMetadataEventType_SET,
			},
		},
	})
	update = sc.drainPendingUpdate()
	assert.False(t, update.updateIsFullState)
	assert.Contains(t, update.updatedKueueWorkloads, "default/job-b")
	assert.Contains(t, sc.kueueWorkloads, "default/job-b")
	assert.Contains(t, sc.kueueWorkloads, "default/job-a")

	// Incremental UNSET removes it from the cache but still reports it as updated.
	sc.applyResponse(&pb.KubeMetadataStreamResponse{
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace: "default",
				Name:      "job-b",
				Type:      pb.KubeMetadataEventType_UNSET,
			},
		},
	})
	update = sc.drainPendingUpdate()
	assert.Contains(t, update.updatedKueueWorkloads, "default/job-b")
	assert.NotContains(t, sc.kueueWorkloads, "default/job-b")
	assert.Contains(t, sc.kueueWorkloads, "default/job-a")
}

func TestDCAStreamClient_ApplyResponse_IncrementalWorkloadBeforeFullStateIgnored(t *testing.T) {
	sc := newDCAStreamClient("node-a", nil)
	// Not initialized yet: an incremental workload update must be ignored.
	sc.applyResponse(&pb.KubeMetadataStreamResponse{
		KueueWorkloads: []*pb.KueueWorkload{
			{
				Namespace: "default",
				Name:      "job-a",
				Type:      pb.KubeMetadataEventType_SET,
			},
		},
	})

	assert.False(t, sc.initialized)
	assert.Empty(t, sc.kueueWorkloads)
	update := sc.drainPendingUpdate()
	assert.Empty(t, update.updatedKueueWorkloads)
}

func assertKueueQueues(t *testing.T, wmetaMock workloadmetamock.Mock, expected map[string]expectedKueueQueue) {
	t.Helper()

	entities := wmetaMock.DumpStructured().Entities[string(workloadmeta.KindKubernetesKueueQueue)]
	assert.Len(t, entities, len(expected))

	for _, entity := range entities {
		queue := entity.(*workloadmeta.KubernetesKueueQueue)
		expectedQueue, found := expected[queue.EntityID.ID]
		require.True(t, found)
		assert.Equal(t, expectedQueue.namespace, queue.Namespace)
		assert.Equal(t, expectedQueue.name, queue.Name)
		assert.Equal(t, workloadmeta.KueueLocalQueue, queue.QueueType)
		assert.Equal(t, expectedQueue.clusterQueueName, queue.ClusterQueueName)
		assert.Equal(t, expectedQueue.labels, queue.Labels)
		assert.Equal(t, expectedQueue.annotations, queue.Annotations)
		assert.Equal(t, expectedQueue.uid, queue.UID)
	}
}

func assertKueueResourceFlavors(t *testing.T, wmetaMock workloadmetamock.Mock, expected map[string]expectedKueueResourceFlavor) {
	t.Helper()

	entities := wmetaMock.DumpStructured().Entities[string(workloadmeta.KindKubernetesKueueResourceFlavor)]
	assert.Len(t, entities, len(expected))

	for _, entity := range entities {
		flavor := entity.(*workloadmeta.KubernetesKueueResourceFlavor)
		expectedFlavor, found := expected[flavor.EntityID.ID]
		require.True(t, found)
		assert.Equal(t, expectedFlavor.name, flavor.Name)
		assert.Equal(t, expectedFlavor.nodeAffinityLabels, flavor.NodeAffinityLabels)
		assert.Equal(t, expectedFlavor.labels, flavor.Labels)
		assert.Equal(t, expectedFlavor.annotations, flavor.Annotations)
		assert.Equal(t, expectedFlavor.uid, flavor.UID)
	}
}

func assertKueueWorkloads(t *testing.T, wmetaMock workloadmetamock.Mock, expected map[string]expectedKueueWorkload) {
	t.Helper()

	entities := wmetaMock.DumpStructured().Entities[string(workloadmeta.KindKubernetesKueueWorkload)]
	assert.Len(t, entities, len(expected))

	for _, entity := range entities {
		workload := entity.(*workloadmeta.KubernetesKueueWorkload)
		expectedWorkload, found := expected[workload.EntityID.ID]
		require.True(t, found)
		assert.Equal(t, expectedWorkload.namespace, workload.Namespace)
		assert.Equal(t, expectedWorkload.name, workload.Name)
		assert.Equal(t, expectedWorkload.queueName, workload.QueueName)
		assert.Equal(t, expectedWorkload.clusterQueueName, workload.ClusterQueueName)
		assert.Equal(t, expectedWorkload.labels, workload.Labels)
		assert.Equal(t, expectedWorkload.annotations, workload.Annotations)
		assert.Equal(t, expectedWorkload.uid, workload.UID)
		assert.Equal(t, expectedWorkload.podSetAssignments, workload.PodSetAssignments)
	}
}

func makePodBundle(events ...workloadmeta.Event) workloadmeta.EventBundle {
	return workloadmeta.EventBundle{
		Events: events,
		Ch:     make(chan struct{}),
	}
}

func makePodSetEvent(uid, namespace, name string, ready bool) workloadmeta.Event {
	return workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   uid,
			},
			EntityMeta: workloadmeta.EntityMeta{
				Name:      name,
				Namespace: namespace,
			},
			Ready: ready,
		},
	}
}

func makePodUnsetEvent(uid string) workloadmeta.Event {
	return workloadmeta.Event{
		Type: workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.KubernetesPod{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindKubernetesPod,
				ID:   uid,
			},
		},
	}
}

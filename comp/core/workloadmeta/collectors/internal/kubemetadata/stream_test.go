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
)

type expectedPod struct {
	services      []string
	nsLabels      map[string]string
	nsAnnotations map[string]string
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

		update       streamUpdate
		expectedPods map[string]expectedPod
	}{
		{
			name: "full state re-enriches all seen pods",
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
			update:          streamUpdate{updateIsFullState: true},
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

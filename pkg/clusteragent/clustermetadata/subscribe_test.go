// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kubernetes "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
)

const testTimeout = 5 * time.Second

// streamFixture returns a store owning node-a (synced) with one pod on it,
// and a node list the test can mutate to force ownership changes.
func streamFixture(t *testing.T) (*LocalStore, func([]string)) {
	t.Helper()
	client := k8sfake.NewSimpleClientset()
	manager := NewLeaseManager(func() (kubernetes.Interface, error) { return client, nil },
		func() (string, error) { return "10.0.0.1", nil },
		"datadog", "dca-0", 40*time.Second, 2*time.Hour)

	nodes := []string{"node-a"}
	controller := NewRingController(manager, MemberID("datadog", "dca-0"), time.Second,
		func(ctx context.Context) ([]string, error) {
			return nodes, nil
		})
	require.NoError(t, controller.Reconcile(context.Background()))
	for _, node := range controller.State().MyNodes {
		controller.SetNodeSynced(node, true)
	}

	podEntity := taggertypes.NewEntityID(taggertypes.KubernetesPodUID, "uid-1")
	wmeta := &fakeWmeta{
		pods: map[string]*workloadmeta.KubernetesPod{
			"uid-1": {
				EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-1"},
				EntityMeta: workloadmeta.EntityMeta{Name: "web-1", Namespace: "default"},
				NodeName:   "node-a",
			},
		},
		deployments: map[string]*workloadmeta.KubernetesDeployment{},
		nodes: map[string]*workloadmeta.KubernetesNode{
			"node-a": {
				EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesNode, ID: "node-a"},
				EntityMeta: workloadmeta.EntityMeta{Name: "node-a"},
			},
		},
	}
	tagger := &fakeTagger{tags: map[taggertypes.EntityID][]string{
		podEntity: {"kube_namespace:default", "pod_name:web-1"},
	}}

	return NewLocalStore(wmeta, tagger, controller, nil), func(next []string) { nodes = next }
}

// TestSubscribeDenials tests the refusal cases.
// Test partitions:
// - node: owned | not owned | owned but unsynced
func TestSubscribeDenials(t *testing.T) {
	store, setNodes := streamFixture(t)
	ctx := context.Background()

	_, _, err := store.Subscribe(ctx, "node-b", cm.Scope{Consumer: "test"})
	assert.Error(t, err, "a non-owner refuses")

	store.ring.SetNodeSynced("node-a", false)
	_, _, err = store.Subscribe(ctx, "node-a", cm.Scope{Consumer: "test"})
	assert.Error(t, err, "an unsynced node is refused with retry semantics")
	store.ring.SetNodeSynced("node-a", true)

	setNodes(nil) // keep the closure honest; no effect until reconcile
	_, _, err = store.Subscribe(ctx, "node-a", cm.Scope{Consumer: "test"})
	assert.NoError(t, err, "owned and synced nodes are accepted")
}

// TestSubscribeStream tests the stream lifecycle.
// Test partitions:
// - burst: current pods of the node delivered
// - changes: set and unset events routed by node
// - end: ownership loss closes the stream
func TestSubscribeStream(t *testing.T) {
	store, setNodes := streamFixture(t)
	ctx := context.Background()

	events, cancel, err := store.Subscribe(ctx, "node-a", cm.Scope{Consumer: "test"})
	require.NoError(t, err)
	defer cancel()

	// Burst: the pod on node-a, with tags.
	select {
	case event := <-events:
		assert.Equal(t, cm.NodeEvent{
			Kind:      KindPod,
			Namespace: "default",
			Name:      "web-1",
			Tags:      []string{"kube_namespace:default", "pod_name:web-1"},
		}, event)
	case <-time.After(testTimeout):
		t.Fatal("no burst event")
	}

	// Change: a new pod on node-a arrives through the store subscription.
	newPod := &workloadmeta.KubernetesPod{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-2"},
		EntityMeta: workloadmeta.EntityMeta{Name: "web-2", Namespace: "default"},
		NodeName:   "node-a",
	}
	store.wmeta.(*fakeWmeta).pods["uid-2"] = newPod
	store.wmeta.(*fakeWmeta).notifyBundle(workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: newPod,
	})

	select {
	case event := <-events:
		assert.Equal(t, "web-2", event.Name)
		assert.False(t, event.Deleted)
	case <-time.After(testTimeout):
		t.Fatal("no set event")
	}

	// Unset: the pod goes away; the bare entity is named from memory.
	delete(store.wmeta.(*fakeWmeta).pods, "uid-2")
	store.wmeta.(*fakeWmeta).notifyBundle(workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-2"}},
	})

	select {
	case event := <-events:
		assert.True(t, event.Deleted)
		assert.Equal(t, "web-2", event.Name)
	case <-time.After(testTimeout):
		t.Fatal("no unset event")
	}

	// Ownership loss: the stream closes.
	setNodes(nil)
	require.NoError(t, store.ring.Reconcile(ctx))

	select {
	case _, ok := <-events:
		assert.False(t, ok, "the stream closes when this replica loses the node")
	case <-time.After(testTimeout):
		t.Fatal("stream did not close on ownership loss")
	}
}

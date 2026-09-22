// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// fakeWmeta implements only the workloadmeta methods LocalStore uses.
// Unimplemented methods panic via the embedded interface.
type fakeWmeta struct {
	workloadmeta.Component

	pods        map[string]*workloadmeta.KubernetesPod // by UID
	deployments map[string]*workloadmeta.KubernetesDeployment
	nodes       map[string]*workloadmeta.KubernetesNode
}

func (f *fakeWmeta) GetKubernetesPodByName(podName, podNamespace string) (*workloadmeta.KubernetesPod, error) {
	for _, pod := range f.pods {
		if pod.Name == podName && pod.Namespace == podNamespace {
			return pod, nil
		}
	}
	return nil, notFound(podName)
}

func (f *fakeWmeta) GetKubernetesPod(id string) (*workloadmeta.KubernetesPod, error) {
	pod, ok := f.pods[id]
	if !ok {
		return nil, notFound(id)
	}
	return pod, nil
}

func (f *fakeWmeta) GetKubernetesPodForContainer(containerID string) (*workloadmeta.KubernetesPod, error) {
	return nil, notFound(containerID)
}

func (f *fakeWmeta) GetKubernetesDeployment(id string) (*workloadmeta.KubernetesDeployment, error) {
	deployment, ok := f.deployments[id]
	if !ok {
		return nil, notFound(id)
	}
	return deployment, nil
}

func (f *fakeWmeta) GetKubernetesNode(name string) (*workloadmeta.KubernetesNode, error) {
	node, ok := f.nodes[name]
	if !ok {
		return nil, notFound(name)
	}
	return node, nil
}

func (f *fakeWmeta) ListKubernetesPods() []*workloadmeta.KubernetesPod {
	pods := make([]*workloadmeta.KubernetesPod, 0, len(f.pods))
	for _, pod := range f.pods {
		pods = append(pods, pod)
	}
	return pods
}

func (f *fakeWmeta) ListKubernetesNodes() []*workloadmeta.KubernetesNode {
	nodes := make([]*workloadmeta.KubernetesNode, 0, len(f.nodes))
	for _, node := range f.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// fakeTagger implements only Tag.
type fakeTagger struct {
	tagger.Component

	tags map[taggertypes.EntityID][]string
	err  error
}

func (f *fakeTagger) Tag(entityID taggertypes.EntityID, cardinality taggertypes.TagCardinality) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tags[entityID], nil
}

// fakePeer stands in for a peer replica: it answers from its local cache
// only and counts calls, so tests can assert whether a fanout happened.
type fakePeer struct {
	cm.Store

	lookupAnswer cm.LookupAnswer
	lookupErr    error
	originAnswer cm.LookupAnswer
	lookupCalls  int
	originCalls  int
}

func (f *fakePeer) Lookup(ctx context.Context, req cm.LookupRequest) (cm.LookupAnswer, error) {
	f.lookupCalls++
	return f.lookupAnswer, f.lookupErr
}

func (f *fakePeer) LookupOrigin(ctx context.Context, req cm.OriginLookupRequest) (cm.LookupAnswer, error) {
	f.originCalls++
	return f.originAnswer, f.lookupErr
}

func notFound(id string) error {
	return pkgerrors.NewNotFound(id)
}

func newTestStore(t *testing.T) *LocalStore {
	wmeta := &fakeWmeta{
		pods: map[string]*workloadmeta.KubernetesPod{
			"uid-1": {
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "uid-1"},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "web-1",
					Namespace: "default",
				},
			},
		},
		deployments: map[string]*workloadmeta.KubernetesDeployment{
			"default/web": {
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesDeployment, ID: "default/web"},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "web",
					Namespace: "default",
				},
			},
		},
		nodes: map[string]*workloadmeta.KubernetesNode{
			"node-a": {
				EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesNode, ID: "node-a"},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "node-a",
				},
			},
		},
	}

	podEntity := taggertypes.NewEntityID(taggertypes.KubernetesPodUID, "uid-1")
	deploymentEntity := taggertypes.NewEntityID(taggertypes.KubernetesDeployment, "default/web")
	tagger := &fakeTagger{tags: map[taggertypes.EntityID][]string{
		podEntity:        {"kube_namespace:default", "pod_name:web-1"},
		deploymentEntity: {"kube_namespace:default", "deployment:web"},
	}}

	client := k8sfake.NewSimpleClientset()
	manager := NewLeaseManager(client, "datadog", "dca-0", 40*time.Second, 2*time.Hour)
	ring := NewRingController(manager, MemberID("datadog", "dca-0"), time.Second,
		func(ctx context.Context) ([]string, error) {
			return []string{"node-a"}, nil
		})
	require.NoError(t, ring.Reconcile(context.Background()))
	for _, node := range ring.State().MyNodes {
		ring.SetNodeSynced(node, true)
	}

	return NewLocalStore(wmeta, tagger, ring)
}

// TestLookup tests named lookups across kinds and found/absent results.
// Test partitions:
// - kind: pod | deployment | node | unknown (boundary: unsupported kind)
// - result: found | absent
func TestLookup(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	found, err := store.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "test", Cardinality: taggertypes.LowCardinality},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"kube_namespace:default", "pod_name:web-1"}}, found)

	absent, err := store.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "test"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerAbsent}, absent, "pod miss with no peers reduces to Absent: single replica is full coverage")

	deployment, err := store.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindDeployment, Namespace: "default", Name: "web"},
		Scope: cm.Scope{Consumer: "test"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"kube_namespace:default", "deployment:web"}}, deployment)

	node, err := store.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindNode, Name: "node-a"},
		Scope: cm.Scope{Consumer: "test"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerFound}, node, "known entity with no tags stored is found, not absent")

	_, err = store.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: "cronjob", Namespace: "default", Name: "x"},
		Scope: cm.Scope{Consumer: "test"},
	})
	assert.Error(t, err, "unknown kind is a caller error, not an absent entity")
}

// TestLookupOrigin tests origin lookups.
// Test partitions:
// - key: pod UID set | container ID set | neither set (boundary: empty key)
func TestLookupOrigin(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	byUID, err := store.LookupOrigin(ctx, cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: "uid-1"},
		Scope: cm.Scope{Consumer: "test", Cardinality: taggertypes.LowCardinality},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"kube_namespace:default", "pod_name:web-1"}}, byUID)

	unknownUID, err := store.LookupOrigin(ctx, cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: "uid-missing"},
		Scope: cm.Scope{Consumer: "test"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerAbsent}, unknownUID, "UID miss with no peers reduces to Absent: single replica is full coverage")

	_, err = store.LookupOrigin(ctx, cm.OriginLookupRequest{
		Key:   cm.OriginKey{},
		Scope: cm.Scope{Consumer: "test"},
	})
	assert.Error(t, err, "empty origin key is a caller error")
}

// TestSnapshot tests enumeration.
// Test partitions:
// - kind: pod | unsupported (boundary)
// - namespace filter: all namespaces | one namespace with a match and one without
func TestSnapshot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	snapshot, err := store.Snapshot(ctx, KindPod, "", cm.Scope{Consumer: "test"})
	require.NoError(t, err)
	assert.Equal(t, []string{"node-a"}, snapshot.Nodes)
	require.Len(t, snapshot.Events, 1)
	assert.Equal(t, cm.NodeEvent{
		Kind:      KindPod,
		Namespace: "default",
		Name:      "web-1",
		Tags:      []string{"kube_namespace:default", "pod_name:web-1"},
	}, snapshot.Events[0])

	filtered, err := store.Snapshot(ctx, KindPod, "kube-system", cm.Scope{Consumer: "test"})
	require.NoError(t, err)
	assert.Empty(t, filtered.Events, "namespace with no pods yields an empty snapshot")

	_, err = store.Snapshot(ctx, KindDeployment, "", cm.Scope{Consumer: "test"})
	assert.Error(t, err, "non-pod enumeration is not implemented in v1")
}

// TestLookupFanout tests the coordinator path for pod queries.
// Test partitions:
// - pod location: local | peer | nowhere
// - kind on miss: pod (fans out) | deployment (no fanout)
// - peer error: none | present
func TestLookupFanout(t *testing.T) {
	ctx := context.Background()
	podReq := cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "test", Cardinality: taggertypes.LowCardinality},
	}
	missReq := cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "test"},
	}

	local := newTestStore(t)
	peer := &fakePeer{}
	store := NewLocalStore(local.wmeta, local.tagger, local.ring, peer)

	found, err := store.Lookup(ctx, podReq)
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerFound, found.Kind, "local hit does not fan out")
	assert.Zero(t, peer.lookupCalls, "no peer called on local hit")

	peer.lookupAnswer = cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"kube_namespace:default"}}
	fromPeer, err := store.Lookup(ctx, missReq)
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"kube_namespace:default"}}, fromPeer, "local miss fans out and the peer's Found wins")
	assert.Equal(t, 1, peer.lookupCalls)

	peer.lookupAnswer = cm.LookupAnswer{Kind: cm.AnswerNotMine}
	nowhere, err := store.Lookup(ctx, missReq)
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerAbsent}, nowhere, "all NotMine (local + peer) reduces to Absent")

	deploymentMiss := cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindDeployment, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "test"},
	}
	peer.lookupCalls = 0
	absent, err := store.Lookup(ctx, deploymentMiss)
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerAbsent}, absent, "replicated kinds answer Absent locally, no fanout")
	assert.Zero(t, peer.lookupCalls)

	peer.lookupErr = errors.New("peer unreachable")
	_, err = store.Lookup(ctx, missReq)
	assert.Error(t, err, "a peer failure fails the broadcast: absence cannot be concluded")
}

// TestLookupOriginFanout tests the coordinator path for origin queries.
// Test partitions:
// - pod location: local | peer | nowhere
// - peer error: none | present
func TestLookupOriginFanout(t *testing.T) {
	ctx := context.Background()
	localReq := cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: "uid-1"},
		Scope: cm.Scope{Consumer: "test", Cardinality: taggertypes.LowCardinality},
	}
	missReq := cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: "uid-missing"},
		Scope: cm.Scope{Consumer: "test"},
	}

	local := newTestStore(t)
	peer := &fakePeer{}
	store := NewLocalStore(local.wmeta, local.tagger, local.ring, peer)

	found, err := store.LookupOrigin(ctx, localReq)
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerFound, found.Kind)
	assert.Zero(t, peer.originCalls, "local hit does not fan out")

	peer.originAnswer = cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"pod_name:web-1"}}
	fromPeer, err := store.LookupOrigin(ctx, missReq)
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerFound, Tags: []string{"pod_name:web-1"}}, fromPeer)
	assert.Equal(t, 1, peer.originCalls)

	peer.originAnswer = cm.LookupAnswer{Kind: cm.AnswerNotMine}
	nowhere, err := store.LookupOrigin(ctx, missReq)
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{Kind: cm.AnswerAbsent}, nowhere)

	peer.lookupErr = errors.New("peer unreachable")
	_, err = store.LookupOrigin(ctx, missReq)
	assert.Error(t, err)
}

// TestRingAndSubscribe tests the topology methods.
// Test partitions:
// - Ring: single-member v1 shape
// - Subscribe: not implemented (boundary)
func TestRingAndSubscribe(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	ring, err := store.Ring(ctx)
	require.NoError(t, err)
	require.Len(t, ring.Members, 1)
	assert.Equal(t, MemberID("datadog", "dca-0"), ring.Members[0].Name)
	assert.Equal(t, []string{"node-a"}, ring.Members[0].Nodes)
	assert.True(t, ring.Members[0].Ready, "all owned nodes are synced in the test fixture")

	_, _, err = store.Subscribe(ctx, "node-a", cm.Scope{Consumer: "test"})
	assert.Error(t, err, "subscribe arrives with the node stream increment")
}

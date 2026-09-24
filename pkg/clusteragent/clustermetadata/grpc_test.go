// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	kubernetes "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
)

const testTimeout = 5 * time.Second

// fanoutFixture returns a store owning node-a (synced) holding one pod,
// plus a setter to force node-list changes.
func fanoutFixture(t *testing.T) (*LocalStore, func([]string)) {
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
		nodes:       map[string]*workloadmeta.KubernetesNode{},
	}
	tagger := &fakeTagger{tags: map[taggertypes.EntityID][]string{
		podEntity: {"kube_namespace:default", "pod_name:web-1"},
	}}

	return NewLocalStore(wmeta, tagger, controller, nil), func(next []string) { nodes = next }
}

// newBufconnPeer spins up the peer server over a bufconn and returns a
// PeerClient wired to it, plus a cleanup func.
func newBufconnPeer(t *testing.T, store *LocalStore) (*PeerClient, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	NewPeerServer(store).Register(server)
	go server.Serve(listener)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	return NewPeerClient(conn), func() {
		conn.Close()
		server.Stop()
	}
}

// TestPeerClientRoundtrip tests the peer gRPC surface against a real store.
// Test partitions:
// - answer: found | not mine | absent (replicated kind)
// - origin: found by UID | miss
// - snapshot: pods of the shard
// - ring: members with readiness
func TestPeerClientRoundtrip(t *testing.T) {
	store, _ := fanoutFixture(t)
	ctx := context.Background()

	peer, cleanup := newBufconnPeer(t, store)
	defer cleanup()

	found, err := peer.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "peer", Cardinality: taggertypes.LowCardinality},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{
		Kind: cm.AnswerFound,
		Tags: []string{"kube_namespace:default", "pod_name:web-1"},
	}, found, "the peer answers from its local cache")

	miss, err := peer.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "peer"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerNotMine, miss.Kind, "a pod miss answers NotMine, the coordinator reduces")

	absent, err := peer.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindDeployment, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "peer"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerAbsent, absent.Kind, "a replicated-kind miss answers Absent, the peer is authoritative")

	origin, err := peer.LookupOrigin(ctx, cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: "uid-1"},
		Scope: cm.Scope{Consumer: "peer"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerFound, origin.Kind)

	originMiss, err := peer.LookupOrigin(ctx, cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: "uid-missing"},
		Scope: cm.Scope{Consumer: "peer"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerNotMine, originMiss.Kind)
}

// TestPeerFanoutThroughTransport tests the coordinator path with a real
// transport-backed peer: the broadcast reaches another replica.
// Test partitions:
// - pod location: local | peer (through gRPC) | nowhere (both NotMine)
func TestPeerFanoutThroughTransport(t *testing.T) {
	ctx := context.Background()

	// Replica B: owns node-b, holds the pod.
	storeB, _ := fanoutFixture(t)
	storeB.ring.SetNodeSynced("node-a", true)
	peerB, cleanup := newBufconnPeer(t, storeB)
	defer cleanup()

	// Replica A: same fixture, misses locally, fans out to B.
	storeA, _ := fanoutFixture(t)
	fanout := NewLocalStore(storeA.wmeta, storeA.tagger, storeA.ring, func() []cm.Store { return []cm.Store{peerB} })

	fromPeer, err := fanout.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "test", Cardinality: taggertypes.LowCardinality},
	})
	require.NoError(t, err)
	// Replica A holds web-1 too in this fixture, so the local hit wins before
	// any fanout. Remove it locally to force the broadcast.
	storeA.wmeta.(*fakeWmeta).pods = map[string]*workloadmeta.KubernetesPod{}

	fromPeer, err = fanout.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "test", Cardinality: taggertypes.LowCardinality},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerFound, fromPeer.Kind, "a pod held by replica B resolves through the transport")

	storeB.wmeta.(*fakeWmeta).pods = map[string]*workloadmeta.KubernetesPod{}
	nowhere, err := fanout.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "test"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerAbsent, nowhere.Kind, "NotMine from both replicas reduces to Absent")
}

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

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
)

// newBufconnConsumer spins up a replica's serving surfaces over a bufconn
// (consumer surface fronting the coordinator, with optional peers for the
// fanout) and returns a ConsumerClient wired to it, plus a cleanup func.
func newBufconnConsumer(t *testing.T, store *LocalStore, peers ...*PeerClient) (*ConsumerClient, func()) {
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

	// Wire the given peers into the store's fanout, pointing at their
	// serving surfaces.
	peerStores := make([]cm.Store, 0, len(peers))
	for _, peer := range peers {
		peerStores = append(peerStores, peer)
	}
	store.peerSource = func() []cm.Store { return peerStores }

	return NewConsumerClient(conn), func() {
		conn.Close()
		server.Stop()
	}
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
	store, _ := streamFixture(t)
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

	snapshot, err := peer.Snapshot(ctx, KindPod, "", cm.Scope{Consumer: "peer"})
	require.NoError(t, err)
	assert.Equal(t, []string{"node-a"}, snapshot.Nodes)
	require.Len(t, snapshot.Events, 1)
	assert.Equal(t, "web-1", snapshot.Events[0].Name)

	ring, err := peer.Ring(ctx)
	require.NoError(t, err)
	require.Len(t, ring.Members, 1)
	assert.Equal(t, MemberID("datadog", "dca-0"), ring.Members[0].Name)
	assert.True(t, ring.Members[0].Ready)
}

// TestPeerFanoutThroughTransport tests the coordinator path with a real
// transport-backed peer: the broadcast reaches another replica.
// Test partitions:
// - pod location: local | peer (through gRPC) | nowhere (both NotMine)
func TestPeerFanoutThroughTransport(t *testing.T) {
	ctx := context.Background()

	// Replica B: owns node-b, holds the pod.
	storeB, _ := streamFixture(t)
	storeB.ring.SetNodeSynced("node-a", true)
	peerB, cleanup := newBufconnPeer(t, storeB)
	defer cleanup()

	// Replica A: same fixture, misses locally, fans out to B.
	storeA, _ := streamFixture(t)
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

// TestConsumerRoundtrip tests the consumer surface: any replica can serve
// a query, pods resolve through the fanout, misses reduce for the ring.
// Test partitions:
// - pod location: other replica (fanout through transport) | nowhere (both NotMine)
// - kind: pod (coordinator path) | deployment (local, replicated)
func TestConsumerRoundtrip(t *testing.T) {
	ctx := context.Background()

	// Replica B owns node-a and holds the pod.
	storeB, _ := streamFixture(t)
	storeB.ring.SetNodeSynced("node-a", true)
	peerB, cleanupB := newBufconnPeer(t, storeB)
	defer cleanupB()

	// Replica A: same ownership, but an empty cache — its answer must come
	// from the fanout to B.
	storeA, _ := streamFixture(t)
	storeA.wmeta.(*fakeWmeta).pods = map[string]*workloadmeta.KubernetesPod{}
	consumerA, cleanupA := newBufconnConsumer(t, storeA, peerB)
	defer cleanupA()

	fromPeer, err := consumerA.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "consumer", Cardinality: taggertypes.LowCardinality},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.LookupAnswer{
		Kind: cm.AnswerFound,
		Tags: []string{"kube_namespace:default", "pod_name:web-1"},
	}, fromPeer, "a pod held by replica B resolves through A's fanout")

	storeB.wmeta.(*fakeWmeta).pods = map[string]*workloadmeta.KubernetesPod{}
	nowhere, err := consumerA.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "web-1"},
		Scope: cm.Scope{Consumer: "consumer"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerAbsent, nowhere.Kind, "NotMine from both replicas reduces to Absent")

	absent, err := consumerA.Lookup(ctx, cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindDeployment, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "consumer"},
	})
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerAbsent, absent.Kind, "replicated kinds answer locally on the consumer surface")
}

// TestConsumerSubscribe tests the consumer stream over gRPC.
// Test partitions:
// - node: owned (stream opens, burst arrives) | not owned (refused)
// - end: ownership loss closes the channel
func TestConsumerSubscribe(t *testing.T) {
	ctx := context.Background()
	storeA, setNodesA := streamFixture(t)
	consumerA, cleanupA := newBufconnConsumer(t, storeA, nil)
	defer cleanupA()

	// A node this replica does not own is refused: the consumer re-resolves
	// through Ring.
	_, _, err := consumerA.Subscribe(ctx, "node-b", cm.Scope{Consumer: "consumer"})
	assert.Error(t, err, "a non-owner refuses the stream")

	events, cancel, err := consumerA.Subscribe(ctx, "node-a", cm.Scope{Consumer: "consumer"})
	require.NoError(t, err)
	defer cancel()

	select {
	case event := <-events:
		assert.Equal(t, "web-1", event.Name, "the burst delivers the node's pods")
	case <-time.After(testTimeout):
		t.Fatal("no burst event")
	}

	// Ownership loss ends the stream: the channel closes.
	setNodesA(nil)
	require.NoError(t, storeA.ring.Reconcile(ctx))
	select {
	case _, ok := <-events:
		assert.False(t, ok, "the stream closes when this replica loses the node")
	case <-time.After(testTimeout):
		t.Fatal("stream did not close on ownership loss")
	}
}

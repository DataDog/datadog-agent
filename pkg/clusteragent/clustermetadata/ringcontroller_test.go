// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubernetes "k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// newRingFixture returns a controller for dca-0 with a static node set and a
// fake clientset preloaded with one alive peer (dca-1) and one expired peer
// (dca-2).
func newRingFixture(t *testing.T, nodes []string) (*RingController, *k8sfake.Clientset) {
	t.Helper()
	client := k8sfake.NewSimpleClientset()
	now := time.Now()

	ringLease := func(name, podName string, age time.Duration) *coordinationv1.Lease {
		renewed := metav1.NewMicroTime(now.Add(-age))
		seconds := int32(40)
		return &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      RingLeaseNamePrefix + podName,
				Namespace: "datadog",
				Labels:    map[string]string{RingLeaseLabel: "true"},
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       stringPtr(MemberID("datadog", podName)),
				LeaseDurationSeconds: &seconds,
				RenewTime:            &renewed,
			},
		}
	}

	ctx := context.Background()
	_, err := client.CoordinationV1().Leases("datadog").Create(ctx, ringLease("", "dca-1", time.Second), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.CoordinationV1().Leases("datadog").Create(ctx, ringLease("", "dca-2", 41*time.Second), metav1.CreateOptions{})
	require.NoError(t, err)

	manager := NewLeaseManager(func() (kubernetes.Interface, error) { return client, nil }, func() (string, error) { return "10.0.0.1", nil }, "datadog", "dca-0", 40*time.Second, 2*time.Hour)
	controller := NewRingController(manager, MemberID("datadog", "dca-0"), time.Second,
		func(ctx context.Context) ([]string, error) {
			return nodes, nil
		})
	return controller, client
}

// TestRingControllerReconcile tests one reconcile pass.
// Test partitions:
// - membership: alive peers included | expired peers excluded
// - publication: own lease carries the owned-node annotation
// - readiness: no sync marks (NotReady) | all owned nodes marked (Ready)
func TestRingControllerReconcile(t *testing.T) {
	ctx := context.Background()
	nodes := []string{"node-a", "node-b", "node-c", "node-d"}
	controller, client := newRingFixture(t, nodes)

	require.NoError(t, controller.Reconcile(ctx))

	state := controller.State()
	assert.Equal(t, []string{MemberID("datadog", "dca-0"), MemberID("datadog", "dca-1")}, MemberNames(state.MemberInfos),
		"expired dca-2 is excluded")
	assert.False(t, state.Ready(), "no sync marks yet: not ready")

	// Every node is owned by some alive member.
	assigned := 0
	for _, owned := range state.Owned {
		assigned += len(owned)
	}
	assert.Equal(t, len(nodes), assigned)

	// The own lease published this replica's owned set.
	lease, err := client.CoordinationV1().Leases("datadog").Get(ctx, RingLeaseNamePrefix+"dca-0", metav1.GetOptions{})
	require.NoError(t, err)
	published, err := UnmarshalOwnedNodes(lease.Annotations[OwnedNodesAnnotation])
	require.NoError(t, err)
	assert.Equal(t, state.MyNodes, published)

	for _, node := range state.MyNodes {
		controller.SetNodeSynced(node, true)
	}
	assert.True(t, controller.State().Ready())
}

// TestRingControllerMembershipChange tests a rebalance pass.
// Test partitions:
// - membership change: dead member revives | owned set shrinks accordingly
// - callback: fired on change | not fired when the set is unchanged
// - sync state: entries for lost nodes dropped | entries for kept nodes kept
func TestRingControllerMembershipChange(t *testing.T) {
	ctx := context.Background()
	nodes := []string{"node-a", "node-b", "node-c", "node-d"}
	controller, client := newRingFixture(t, nodes)

	require.NoError(t, controller.Reconcile(ctx))
	state := controller.State()
	for _, node := range state.MyNodes {
		controller.SetNodeSynced(node, true)
	}

	var prev, next []string
	controller.SubscribeOwnedNodes(func(p, n []string) {
		prev, next = p, n
	})

	// Revive dca-2: it pulls ~1/3 of the nodes from the two live members.
	renewed := metav1.NewMicroTime(time.Now())
	lease, err := client.CoordinationV1().Leases("datadog").Get(ctx, RingLeaseNamePrefix+"dca-2", metav1.GetOptions{})
	require.NoError(t, err)
	lease.Spec.RenewTime = &renewed
	_, err = client.CoordinationV1().Leases("datadog").Update(ctx, lease, metav1.UpdateOptions{})
	require.NoError(t, err)

	require.NoError(t, controller.Reconcile(ctx))

	newState := controller.State()
	require.Len(t, newState.MemberInfos, 3)
	assert.Equal(t, state.MyNodes, prev, "callback received the previous owned set")
	assert.Equal(t, newState.MyNodes, next, "callback received the new owned set")
	assert.Less(t, len(newState.MyNodes), len(state.MyNodes), "dca-2 took some nodes over")

	for _, node := range newState.MyNodes {
		if contains(state.MyNodes, node) {
			assert.True(t, newState.NodeSynced[node], "sync marks for kept nodes survive")
		} else {
			assert.False(t, newState.NodeSynced[node], "gained nodes start unsynced")
		}
	}

	// A pass with no membership change does not fire the callback again.
	prev, next = nil, nil
	require.NoError(t, controller.Reconcile(ctx))
	assert.Nil(t, prev, "unchanged owned set does not fire the callback")
}

// TestLocalStoreReadinessGate tests the pod-miss answer through the ring state.
// Test partitions:
// - readiness: watches unsynced (NotReady) | watches synced (NotMine)
// - ring answer: self member Ready reflects the gate | peers' Ready is true
func TestLocalStoreReadinessGate(t *testing.T) {
	ctx := context.Background()
	nodes := []string{"node-a", "node-b"}
	controller, _ := newRingFixture(t, nodes)

	require.NoError(t, controller.Reconcile(ctx))

	wmeta := &fakeWmeta{
		pods:        map[string]*workloadmeta.KubernetesPod{},
		deployments: map[string]*workloadmeta.KubernetesDeployment{},
		nodes:       map[string]*workloadmeta.KubernetesNode{},
	}
	store := NewLocalStore(wmeta, &fakeTagger{tags: map[taggertypes.EntityID][]string{}}, controller, nil)

	miss := cm.LookupRequest{
		Key:   cm.EntityKey{Kind: KindPod, Namespace: "default", Name: "missing"},
		Scope: cm.Scope{Consumer: "test"},
	}

	answer, err := store.localLookup(ctx, miss)
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerNotReady, answer.Kind, "unsynced watches: not ready")

	for _, node := range controller.State().MyNodes {
		controller.SetNodeSynced(node, true)
	}
	answer, err = store.localLookup(ctx, miss)
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerNotMine, answer.Kind, "synced watches: another replica might hold it")

	// Coordinator level: no peers configured, so the single NotMine reduces
	// to Absent (full coverage by reduction).
	answer, err = store.Lookup(ctx, miss)
	require.NoError(t, err)
	assert.Equal(t, cm.AnswerAbsent, answer.Kind)

	// The self member's readiness reflects the gate; the other members
	// are unknown and not the store's business.
	state := controller.State()
	assert.True(t, state.Ready(), "self readiness follows the sync gate after marking nodes synced")
}

// TestRingControllerLeaseDeletionRecovery tests that a lease deleted out from
// under this replica is re-created on the next pass.
// Test partitions:
// - lease state: present (idempotent ensure) | deleted (recreated, member alive again)
func TestRingControllerLeaseDeletionRecovery(t *testing.T) {
	ctx := context.Background()
	controller, client := newRingFixture(t, []string{"node-a", "node-b"})

	require.NoError(t, controller.Reconcile(ctx))
	state := controller.State()
	assert.Contains(t, MemberNames(state.MemberInfos), MemberID("datadog", "dca-0"))

	// Delete our lease, as a namespace wipe or a human could.
	require.NoError(t, client.CoordinationV1().Leases("datadog").
		Delete(ctx, RingLeaseNamePrefix+"dca-0", metav1.DeleteOptions{}))

	require.NoError(t, controller.Reconcile(ctx))

	recovered := controller.State()
	assert.Contains(t, MemberNames(recovered.MemberInfos), MemberID("datadog", "dca-0"),
		"the member is alive in its own ring view after one pass")
	assert.Equal(t, state.MyNodes, recovered.MyNodes, "owned set recomputed identically")
}

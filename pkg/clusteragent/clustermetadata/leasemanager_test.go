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

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newManager(t *testing.T) (*LeaseManager, *k8sfake.Clientset) {
	client := k8sfake.NewSimpleClientset()
	manager := NewLeaseManager(client, "datadog", "dca-0", 40*time.Second, 2*time.Hour)
	return manager, client
}

// TestLeaseEnsureAndRenew tests the replica's own Lease lifecycle.
// Test partitions:
// - ensure: missing lease (create) | existing lease (idempotent)
// - renew: first renewal | annotation update (owned set changed)
func TestLeaseEnsureAndRenew(t *testing.T) {
	ctx := context.Background()
	manager, client := newManager(t)

	require.NoError(t, manager.Ensure(ctx))

	lease, err := client.CoordinationV1().Leases("datadog").Get(ctx, RingLeaseNamePrefix+"dca-0", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "true", lease.Labels[RingLeaseLabel])
	require.NotNil(t, lease.Spec.HolderIdentity)
	assert.Equal(t, MemberID("datadog", "dca-0"), *lease.Spec.HolderIdentity)
	require.NotNil(t, lease.Spec.LeaseDurationSeconds)
	assert.EqualValues(t, 40, *lease.Spec.LeaseDurationSeconds)

	renewedAt := *lease.Spec.RenewTime

	// Idempotent: a second Ensure neither fails nor duplicates.
	require.NoError(t, manager.Ensure(ctx))
	list, err := client.CoordinationV1().Leases("datadog").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, list.Items, 1)

	manager.SetOwnedNodes([]string{"node-b", "node-a"})
	require.NoError(t, manager.Renew(ctx))

	lease, err = client.CoordinationV1().Leases("datadog").Get(ctx, RingLeaseNamePrefix+"dca-0", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, lease.Spec.RenewTime.After(renewedAt.Time), "renewal advances RenewTime")
	decoded, err := UnmarshalOwnedNodes(lease.Annotations[OwnedNodesAnnotation])
	require.NoError(t, err)
	assert.Equal(t, []string{"node-a", "node-b"}, decoded, "annotation is canonical (sorted)")
}

// TestLeaseRenewConflictSurfaced tests that an update conflict is not retried.
// Test partitions:
// - renew: with an injected conflict | without conflict
func TestLeaseRenewConflictSurfaced(t *testing.T) {
	ctx := context.Background()

	client := k8sfake.NewSimpleClientset()
	manager := NewLeaseManager(client, "datadog", "dca-0", 40*time.Second, 2*time.Hour)
	require.NoError(t, manager.Ensure(ctx))

	updates := 0
	client.Fake.PrependReactor("update", "leases",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			updates++
			return true, nil, apierrors.NewConflict(
				coordinationv1.Resource("leases"), "dd-metadata-ring-dca-0", nil)
		})

	manager.SetOwnedNodes([]string{"node-a"})
	err := manager.Renew(ctx)
	assert.Error(t, err, "the conflict surfaces to the renewal tick, which retries")
	assert.Equal(t, 1, updates, "exactly one update attempt: no retry")
}

// TestMembers tests member discovery.
// Test partitions:
// - lease labels: ring | non-ring (filtered out)
// - holder identity: set | empty (skipped)
// - liveness: alive | expired (both returned; liveness is the caller's)
func TestMembers(t *testing.T) {
	ctx := context.Background()
	manager, client := newManager(t)
	now := time.Now()

	ringLease := func(name, holder string, age time.Duration, duration time.Duration) *coordinationv1.Lease {
		renewed := metav1.NewMicroTime(now.Add(-age))
		seconds := int32(duration / time.Second)
		return &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "datadog",
				Labels:    map[string]string{RingLeaseLabel: "true"},
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &holder,
				LeaseDurationSeconds: &seconds,
				RenewTime:            &renewed,
			},
		}
	}

	_, err := client.CoordinationV1().Leases("datadog").Create(ctx,
		ringLease("dd-metadata-ring-dca-1", MemberID("datadog", "dca-1"), 1*time.Second, 40*time.Second), metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.CoordinationV1().Leases("datadog").Create(ctx,
		ringLease("dd-metadata-ring-dca-2", MemberID("datadog", "dca-2"), 41*time.Second, 40*time.Second), metav1.CreateOptions{})
	require.NoError(t, err)

	// Ring lease without a holder: skipped, not a member.
	_, err = client.CoordinationV1().Leases("datadog").Create(ctx,
		ringLease("dd-metadata-ring-ghost", "", time.Second, 40*time.Second), metav1.CreateOptions{})
	require.NoError(t, err)

	// Non-ring lease in the same namespace: invisible to the ring.
	_, err = client.CoordinationV1().Leases("datadog").Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-leader-election", Namespace: "datadog"},
		Spec:       coordinationv1.LeaseSpec{},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	members, err := manager.Members(ctx)
	require.NoError(t, err)
	require.Len(t, members, 2)

	alive := AliveMembers(members, now)
	assert.Equal(t, []string{MemberID("datadog", "dca-1")}, alive, "expired dca-2 is excluded by liveness, not by listing")
}

// TestCleanup tests expired-lease retention.
// Test partitions:
// - lease state: alive | expired within retention | expired past retention
// - ownership: own lease | other members' (own never deleted)
// - labels: ring | non-ring (never deleted)
func TestCleanup(t *testing.T) {
	ctx := context.Background()
	manager, client := newManager(t)
	now := time.Now()

	require.NoError(t, manager.Ensure(ctx))
	require.NoError(t, manager.Renew(ctx))

	ringLease := func(name, holder string, age time.Duration) *coordinationv1.Lease {
		renewed := metav1.NewMicroTime(now.Add(-age))
		seconds := int32(40)
		return &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "datadog",
				Labels:    map[string]string{RingLeaseLabel: "true"},
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &holder,
				LeaseDurationSeconds: &seconds,
				RenewTime:            &renewed,
			},
		}
	}

	// Expired 1h ago: within the 2h retention window — kept.
	_, err := client.CoordinationV1().Leases("datadog").Create(ctx,
		ringLease("dd-metadata-ring-dca-1", MemberID("datadog", "dca-1"), 1*time.Hour+40*time.Second), metav1.CreateOptions{})
	require.NoError(t, err)
	// Expired 3h ago: past retention — deleted.
	_, err = client.CoordinationV1().Leases("datadog").Create(ctx,
		ringLease("dd-metadata-ring-dca-2", MemberID("datadog", "dca-2"), 3*time.Hour+40*time.Second), metav1.CreateOptions{})
	require.NoError(t, err)
	// Non-ring lease, long dead: never deleted.
	_, err = client.CoordinationV1().Leases("datadog").Create(ctx, &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "other-lease", Namespace: "datadog"},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	require.NoError(t, manager.Cleanup(ctx, now))

	remaining, err := client.CoordinationV1().Leases("datadog").List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	names := map[string]bool{}
	for _, l := range remaining.Items {
		names[l.Name] = true
	}
	assert.True(t, names[RingLeaseNamePrefix+"dca-0"], "own lease is never deleted")
	assert.True(t, names["dd-metadata-ring-dca-1"], "expired within retention is kept as history")
	assert.False(t, names["dd-metadata-ring-dca-2"], "expired past retention is deleted")
	assert.True(t, names["other-lease"], "non-ring leases are never deleted")
}

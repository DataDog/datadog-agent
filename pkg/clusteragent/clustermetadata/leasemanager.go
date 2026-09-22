// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kubernetes "k8s.io/client-go/kubernetes"
)

// RingLeaseLabel marks a Lease as belonging to the metadata ring.
const RingLeaseLabel = "clusteragent.datadoghq.com/metadata-ring"

// RingLeaseNamePrefix is the prefix of every ring member's Lease name.
const RingLeaseNamePrefix = "dd-metadata-ring-"

// MemberID is the member identity: "namespace/pod-name".
func MemberID(namespace, podName string) string {
	return namespace + "/" + podName
}

// LeaseManager owns this replica's ring Lease and observes the others'.
//
// Each replica:
//   - creates and renews one Lease labeled with RingLeaseLabel, holding its
//     member ID ("namespace/pod-name") as holder identity,
//   - publishes its owned-node set in the OwnedNodesAnnotation on renewal,
//   - lists the other members' leases on demand for ring computation,
//   - deletes ring Leases that expired longer than the retention window ago.
type LeaseManager struct {
	client     kubernetes.Interface
	namespace  string
	podName    string
	leaseName  string
	duration   time.Duration
	retention  time.Duration
	ringLabels labels.Set

	mu         sync.RWMutex
	ownedNodes []string
}

// NewLeaseManager returns the manager for this replica.
func NewLeaseManager(client kubernetes.Interface, namespace, podName string, duration, retention time.Duration) *LeaseManager {
	return &LeaseManager{
		client:     client,
		namespace:  namespace,
		podName:    podName,
		leaseName:  RingLeaseNamePrefix + podName,
		duration:   duration,
		retention:  retention,
		ringLabels: labels.Set{RingLeaseLabel: "true"},
	}
}

// SetOwnedNodes sets the node set published in the annotation at the next
// renewal.
func (m *LeaseManager) SetOwnedNodes(nodes []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ownedNodes = nodes
}

// Ensure creates this replica's Lease if it does not exist yet.
func (m *LeaseManager) Ensure(ctx context.Context) error {
	_, err := m.client.CoordinationV1().Leases(m.namespace).Get(ctx, m.leaseName, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	now := metav1.NowMicro()
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.leaseName,
			Namespace: m.namespace,
			Labels:    m.ringLabels,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       stringPtr(MemberID(m.namespace, m.podName)),
			LeaseDurationSeconds: int32Ptr(int32(m.duration / time.Second)),
			RenewTime:            &now,
		},
	}
	_, err = m.client.CoordinationV1().Leases(m.namespace).Create(ctx, lease, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

// Renew refreshes RenewTime and publishes the current owned-node set.
// Conflicts are not retried: the lease name is derived from this pod's name,
// so a competing writer is either a restart overlap of the same member
// (harmless: same holder identity, converged content, last writer wins) or
// an actor we do not defend against. The renewal tick retries anyway.
func (m *LeaseManager) Renew(ctx context.Context) error {
	lease, err := m.client.CoordinationV1().Leases(m.namespace).Get(ctx, m.leaseName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	m.mu.RLock()
	owned := make([]string, len(m.ownedNodes))
	copy(owned, m.ownedNodes)
	m.mu.RUnlock()

	encoded, err := MarshalOwnedNodes(owned)
	if err != nil {
		return err
	}

	now := metav1.NowMicro()
	lease.Spec.RenewTime = &now
	if lease.Labels == nil {
		lease.Labels = map[string]string{}
	}
	lease.Labels[RingLeaseLabel] = "true"
	if lease.Annotations == nil {
		lease.Annotations = map[string]string{}
	}
	lease.Annotations[OwnedNodesAnnotation] = encoded

	_, err = m.client.CoordinationV1().Leases(m.namespace).Update(ctx, lease, metav1.UpdateOptions{})
	return err
}

// Members lists the ring leases (including expired ones!!) and returns their lease state.
func (m *LeaseManager) Members(ctx context.Context) ([]MemberInfo, error) {
	list, err := m.client.CoordinationV1().Leases(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: m.ringLabels.AsSelector().String(),
	})
	if err != nil {
		return nil, err
	}

	members := make([]MemberInfo, 0, len(list.Items))
	for i := range list.Items {
		if info, ok := memberInfoFromLease(&list.Items[i]); ok {
			members = append(members, info)
		}
	}
	return members, nil
}

// Cleanup deletes ring Leases that expired longer than the `retention` window.
func (m *LeaseManager) Cleanup(ctx context.Context, now time.Time) error {
	list, err := m.client.CoordinationV1().Leases(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: m.ringLabels.AsSelector().String(),
	})
	if err != nil {
		return err
	}

	for i := range list.Items {
		lease := &list.Items[i]
		if lease.Name == m.leaseName {
			continue
		}
		info, ok := memberInfoFromLease(lease)
		if !ok {
			continue
		}
		expiredAt := info.RenewedAt.Add(info.Duration)
		if now.Before(expiredAt.Add(m.retention)) {
			continue
		}
		err := m.client.CoordinationV1().Leases(m.namespace).Delete(ctx, lease.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// memberInfoFromLease extracts a MemberInfo from a ring Lease.
func memberInfoFromLease(lease *coordinationv1.Lease) (MemberInfo, bool) {
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		return MemberInfo{}, false
	}

	info := MemberInfo{Name: *lease.Spec.HolderIdentity}
	if lease.Spec.RenewTime != nil {
		info.RenewedAt = lease.Spec.RenewTime.Time
	}
	if lease.Spec.LeaseDurationSeconds != nil {
		info.Duration = time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	}
	return info, true
}

func stringPtr(s string) *string { return &s }

func int32Ptr(i int32) *int32 { return &i }

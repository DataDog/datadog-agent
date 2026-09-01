// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

const deletionTimestampFamily = "kube_pod_deletion_timestamp"

// genFuncWithDeletionTimestamp creates a generate function that emits
// kube_pod_deletion_timestamp (with one metric) when the pod has a deletion
// timestamp, and kube_pod_info otherwise.
func genFuncWithDeletionTimestamp(obj interface{}) []metric.FamilyInterface {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}
	name := "kube_pod_info"
	value := float64(1)
	if pod.DeletionTimestamp != nil && !pod.DeletionTimestamp.IsZero() {
		name = deletionTimestampFamily
		value = float64(pod.DeletionTimestamp.Unix())
	}
	return []metric.FamilyInterface{&metric.Family{
		Name: name,
		Metrics: []*metric.Metric{{
			LabelKeys:   []string{"namespace", "pod", "uid"},
			LabelValues: []string{pod.Namespace, pod.Name, string(pod.UID)},
			Value:       value,
		}},
	}}
}

// newPodStore returns a store that retains the pods marked as being deleted.
func newPodStore() *MetricsStore {
	ms := NewMetricsStore(genFuncWithDeletionTimestamp, "*v1.Pod")
	ms.EnableDeletionRetention(deletionTimestampFamily)
	return ms
}

func makePod(uid, name, namespace string, deletionTimestamp *metav1.Time) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID(uid),
			DeletionTimestamp: deletionTimestamp,
		},
	}
}

// retained returns the families retained for uid, under the store lock.
func retained(ms *MetricsStore, uid string) ([]DDMetricsFam, bool) {
	ms.mutex.RLock()
	defer ms.mutex.RUnlock()

	families, exists := ms.recentlyDeleted[types.UID(uid)]
	return families, exists
}

func TestDelete_RetainsTerminatingPod(t *testing.T) {
	ms := newPodStore()

	delTs := metav1.Unix(1700000000, 0)
	pod := makePod("uid-1", "terminating-pod", "default", &delTs)
	require.NoError(t, ms.Add(pod))

	// Delete the pod — it had a deletion timestamp, so its metrics should be retained.
	require.NoError(t, ms.Delete(pod))

	// The pod should no longer be in the main metrics map.
	ms.mutex.RLock()
	_, exists := ms.metrics[types.UID("uid-1")]
	ms.mutex.RUnlock()
	assert.False(t, exists, "pod should be removed from metrics map")

	families, exists := retained(ms, "uid-1")
	require.True(t, exists, "pod metrics should be retained in recentlyDeleted")
	require.Len(t, families, 1)
	assert.Equal(t, deletionTimestampFamily, families[0].Name)
	assert.Len(t, families[0].ListMetrics, 1)
}

func TestDelete_DoesNotRetainNonTerminatingPod(t *testing.T) {
	ms := newPodStore()

	pod := makePod("uid-2", "running-pod", "default", nil)
	require.NoError(t, ms.Add(pod))

	require.NoError(t, ms.Delete(pod))

	_, exists := retained(ms, "uid-2")
	assert.False(t, exists, "pod without a deletion timestamp should not be retained")
}

// TestDelete_RetentionDisabledByDefault checks that a store which was not given
// a deletion marker family retains nothing, so the other resource types keep
// their previous behaviour.
func TestDelete_RetentionDisabledByDefault(t *testing.T) {
	ms := NewMetricsStore(genFuncWithDeletionTimestamp, "*v1.Pod")

	delTs := metav1.Unix(1700000000, 0)
	pod := makePod("uid-no-retention", "terminating-pod", "default", &delTs)
	require.NoError(t, ms.Add(pod))
	require.NoError(t, ms.Delete(pod))

	ms.mutex.RLock()
	assert.Empty(t, ms.recentlyDeleted)
	ms.mutex.RUnlock()
	assert.Empty(t, ms.DrainRecentlyDeleted())
}

func TestDrainRecentlyDeleted(t *testing.T) {
	ms := newPodStore()

	delTs := metav1.Unix(1700000000, 0)
	pod := makePod("uid-drain", "deleted-pod", "kube-system", &delTs)
	require.NoError(t, ms.Add(pod))
	require.NoError(t, ms.Delete(pod))

	// Drain should return the retained families and empty the map.
	drained := ms.DrainRecentlyDeleted()
	require.Len(t, drained, 1)
	require.Len(t, drained[0], 1)
	fam := drained[0][0]
	assert.Equal(t, deletionTimestampFamily, fam.Name)
	require.Len(t, fam.ListMetrics, 1)
	m := fam.ListMetrics[0]
	assert.Equal(t, "kube-system", m.Labels["namespace"])
	assert.Equal(t, "deleted-pod", m.Labels["pod"])
	assert.Equal(t, "uid-drain", m.Labels["uid"])
	assert.Equal(t, float64(1700000000), m.Val)

	ms.mutex.RLock()
	assert.Nil(t, ms.recentlyDeleted)
	ms.mutex.RUnlock()

	// A second drain should return nothing.
	assert.Empty(t, ms.DrainRecentlyDeleted())
}

func TestDrainRecentlyDeleted_OneEntryPerPod(t *testing.T) {
	ms := newPodStore()

	delTs := metav1.Unix(1700000000, 0)
	for _, uid := range []string{"uid-a", "uid-b"} {
		pod := makePod(uid, "pod-"+uid, "default", &delTs)
		require.NoError(t, ms.Add(pod))
		require.NoError(t, ms.Delete(pod))
	}

	drained := ms.DrainRecentlyDeleted()
	require.Len(t, drained, 2)

	uids := make([]string, 0, 2)
	for _, families := range drained {
		require.Len(t, families, 1)
		require.Len(t, families[0].ListMetrics, 1)
		uids = append(uids, families[0].ListMetrics[0].Labels["uid"])
	}
	assert.ElementsMatch(t, []string{"uid-a", "uid-b"}, uids)
}

func TestClearRecentlyDeleted(t *testing.T) {
	ms := newPodStore()

	delTs := metav1.Unix(1700000000, 0)
	pod := makePod("uid-4", "deleted-pod", "default", &delTs)
	require.NoError(t, ms.Add(pod))
	require.NoError(t, ms.Delete(pod))

	// Verify retention happened.
	_, exists := retained(ms, "uid-4")
	require.True(t, exists)

	ms.ClearRecentlyDeleted()

	ms.mutex.RLock()
	assert.Nil(t, ms.recentlyDeleted)
	ms.mutex.RUnlock()
	assert.Empty(t, ms.DrainRecentlyDeleted())
}

func TestFilterFamilies(t *testing.T) {
	families := []DDMetricsFam{
		{Name: deletionTimestampFamily, ListMetrics: []DDMetric{{Val: 1700000000, Labels: map[string]string{"pod": "p"}}}},
		{Name: "kube_pod_info", ListMetrics: []DDMetric{{Val: 1, Labels: map[string]string{"pod": "p", "node": "n"}}}},
	}

	// Family filter.
	filtered := FilterFamilies(families, func(f DDMetricsFam) bool { return f.Name == "kube_pod_info" }, GetAllMetrics)
	assert.NotContains(t, filtered, deletionTimestampFamily)
	require.Contains(t, filtered, "kube_pod_info")

	// Metric filter: only the metadata-only metrics, whose value is 1.
	filtered = FilterFamilies(families, GetAllFamilies, func(m DDMetric) bool { return m.Val == 1 })
	assert.NotContains(t, filtered, deletionTimestampFamily)
	require.Contains(t, filtered, "kube_pod_info")

	// No filter.
	all := FilterFamilies(families, GetAllFamilies, GetAllMetrics)
	assert.Contains(t, all, deletionTimestampFamily)
	assert.Contains(t, all, "kube_pod_info")
}

func TestReplace_RetainsTerminatingPod(t *testing.T) {
	ms := newPodStore()

	delTs := metav1.Unix(1700000000, 0)
	terminatingPod := makePod("uid-term", "terminating-pod", "default", &delTs)
	runningPod := makePod("uid-running", "running-pod", "default", nil)
	require.NoError(t, ms.Add(terminatingPod))
	require.NoError(t, ms.Add(runningPod))

	// Replace with only the running pod — the terminating pod is gone.
	require.NoError(t, ms.Replace([]interface{}{runningPod}, ""))

	families, exists := retained(ms, "uid-term")
	require.True(t, exists, "terminating pod should be retained during Replace")
	require.Len(t, families, 1)
	assert.Equal(t, deletionTimestampFamily, families[0].Name)

	_, exists = retained(ms, "uid-running")
	assert.False(t, exists, "non-terminating pod should not be retained")
}

func TestReplace_DoesNotRetainPodStillInList(t *testing.T) {
	ms := newPodStore()

	delTs := metav1.Unix(1700000000, 0)
	terminatingPod := makePod("uid-term", "terminating-pod", "default", &delTs)
	require.NoError(t, ms.Add(terminatingPod))

	// Replace with the same pod — it should NOT be retained.
	require.NoError(t, ms.Replace([]interface{}{terminatingPod}, ""))

	ms.mutex.RLock()
	assert.Empty(t, ms.recentlyDeleted)
	ms.mutex.RUnlock()
}

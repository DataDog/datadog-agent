// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

// seriesTagsTestGen generates, per pod, an info family carrying the node and
// a phase family with one series per phase (only the current one at 1).
func seriesTagsTestGen(obj interface{}) []metric.FamilyInterface {
	pod := obj.(*v1.Pod)
	info := metric.Family{Name: "kube_pod_info", Metrics: []*metric.Metric{{
		LabelKeys: []string{"namespace", "pod", "node"}, LabelValues: []string{pod.Namespace, pod.Name, pod.Spec.NodeName}, Value: 1,
	}}}
	phase := metric.Family{Name: "kube_pod_status_phase"}
	for _, p := range []v1.PodPhase{v1.PodPending, v1.PodRunning} {
		value := 0.0
		if pod.Status.Phase == p {
			value = 1
		}
		phase.Metrics = append(phase.Metrics, &metric.Metric{
			LabelKeys: []string{"namespace", "pod", "phase"}, LabelValues: []string{pod.Namespace, pod.Name, string(p)}, Value: value,
		})
	}
	return []metric.FamilyInterface{&info, &phase}
}

// seriesTagsTestFunc shares pod and node tags across the object and gives each
// phase series its own phase tag, like the KSM check does with joins.
func seriesTagsTestFunc(calls *int) SeriesTagsFunc {
	return func(families []DDMetricsFam) *ObjectTags {
		*calls++
		object := &ObjectTags{}
		for _, f := range families {
			for i, m := range f.ListMetrics {
				if node := m.Labels["node"]; node != "" {
					object.Hostname = node
					object.Tags = []string{"pod_name:" + m.Labels["pod"], "node:" + node}
				}
				if phase := m.Labels["phase"]; phase != "" {
					f.ListMetrics[i].ExtraTags = []string{"pod_phase:" + phase}
				}
			}
		}
		return object
	}
}

func seriesTagsTestPod(uid, name, node string, phase v1.PodPhase) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", UID: types.UID(uid)},
		Spec:       v1.PodSpec{NodeName: node},
		Status:     v1.PodStatus{Phase: phase},
	}
}

// pushedPod is what Push returned for one pod: its object tags and hostname and,
// per family, the extra tags of each pushed series.
type pushedPod struct {
	object *ObjectTags
	extras map[string][][]string
}

func pushSeriesTags(t *testing.T, ms *MetricsStore, metricFilter MetricAllow) map[string]pushedPod {
	t.Helper()
	pods := map[string]pushedPod{}
	for name, families := range ms.Push(GetAllFamilies, metricFilter) {
		for _, f := range families {
			pod := f.ListMetrics[0].Labels["pod"]
			p, found := pods[pod]
			if !found {
				p = pushedPod{object: f.Object, extras: map[string][][]string{}}
			}
			assert.Same(t, p.object, f.Object, "all families of an object share one ObjectTags")
			for _, m := range f.ListMetrics {
				p.extras[name] = append(p.extras[name], m.ExtraTags)
			}
			pods[pod] = p
		}
	}
	return pods
}

func TestSeriesTagsFollowObjectLifecycle(t *testing.T) {
	calls := 0
	ms := NewMetricsStore(seriesTagsTestGen, "*v1.Pod")
	ms.SetSeriesTagsFunc(seriesTagsTestFunc(&calls))

	// Add: tags are computed once per object, when it is added.
	require.NoError(t, ms.Add(seriesTagsTestPod("uid-a", "pod-a", "node-1", v1.PodPending)))
	require.NoError(t, ms.Add(seriesTagsTestPod("uid-b", "pod-b", "node-2", v1.PodRunning)))
	assert.Equal(t, 2, calls)

	pods := pushSeriesTags(t, ms, GetAllMetrics)
	require.Contains(t, pods, "pod-a")
	assert.Equal(t, &ObjectTags{Hostname: "node-1", Tags: []string{"pod_name:pod-a", "node:node-1"}}, pods["pod-a"].object)
	assert.Equal(t, [][]string{{"pod_phase:Pending"}, {"pod_phase:Running"}}, pods["pod-a"].extras["kube_pod_status_phase"])
	assert.Equal(t, [][]string{nil}, pods["pod-a"].extras["kube_pod_info"])

	// Reads do not recompute anything.
	pushSeriesTags(t, ms, GetAllMetrics)
	assert.Equal(t, 2, calls)

	// A metric filter keeps each remaining series with its own extra tags.
	onlyActive := func(m DDMetric) bool { return m.Val == 1 }
	pods = pushSeriesTags(t, ms, onlyActive)
	assert.Equal(t, [][]string{{"pod_phase:Pending"}}, pods["pod-a"].extras["kube_pod_status_phase"])
	assert.Equal(t, [][]string{{"pod_phase:Running"}}, pods["pod-b"].extras["kube_pod_status_phase"])

	// Update: tags follow the object.
	require.NoError(t, ms.Update(seriesTagsTestPod("uid-a", "pod-a", "node-3", v1.PodRunning)))
	assert.Equal(t, 3, calls)
	pods = pushSeriesTags(t, ms, onlyActive)
	assert.Equal(t, "node-3", pods["pod-a"].object.Hostname)
	assert.Equal(t, []string{"pod_name:pod-a", "node:node-3"}, pods["pod-a"].object.Tags)
	assert.Equal(t, [][]string{{"pod_phase:Running"}}, pods["pod-a"].extras["kube_pod_status_phase"])

	// Delete: gone with its series.
	require.NoError(t, ms.Delete(seriesTagsTestPod("uid-b", "pod-b", "node-2", v1.PodRunning)))
	pods = pushSeriesTags(t, ms, GetAllMetrics)
	assert.NotContains(t, pods, "pod-b")

	// Replace (relist): vanished objects go, the others are recomputed.
	require.NoError(t, ms.Replace([]interface{}{seriesTagsTestPod("uid-c", "pod-c", "node-4", v1.PodPending)}, ""))
	assert.Equal(t, 4, calls)
	pods = pushSeriesTags(t, ms, GetAllMetrics)
	assert.NotContains(t, pods, "pod-a")
	require.Contains(t, pods, "pod-c")
	assert.Equal(t, "node-4", pods["pod-c"].object.Hostname)
}

func TestSeriesTagsFuncRunsOutsideTheLock(t *testing.T) {
	ms := NewMetricsStore(seriesTagsTestGen, "*v1.Pod")
	ms.SetSeriesTagsFunc(func(_ []DDMetricsFam) *ObjectTags {
		// Would deadlock if called with the store's write lock held.
		ms.Push(GetAllFamilies, GetAllMetrics)
		return &ObjectTags{}
	})
	require.NoError(t, ms.Add(seriesTagsTestPod("uid-a", "pod-a", "node-1", v1.PodRunning)))
}

func TestNoSeriesTagsFunc(t *testing.T) {
	ms := NewMetricsStore(seriesTagsTestGen, "*v1.Pod")
	require.NoError(t, ms.Add(seriesTagsTestPod("uid-a", "pod-a", "node-1", v1.PodRunning)))
	for _, families := range ms.Push(GetAllFamilies, GetAllMetrics) {
		for _, f := range families {
			assert.Nil(t, f.Object)
			for _, m := range f.ListMetrics {
				assert.Nil(t, m.ExtraTags)
			}
		}
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	"fmt"
	"runtime"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

// podMetric returns a metric of the pod o with the namespace, pod and uid
// labels, then keys and values. Like mergeKeyValues of kube-state-metrics, it
// allocates the keys and the values in a single block.
func podMetric(o metav1.Object, keys []string, values ...string) *metric.Metric {
	n := 3 + len(keys)
	keysValues := make([]string, 0, 2*n)
	labelKeys := append(keysValues[0:0:n], "namespace", "pod", "uid")
	labelKeys = append(labelKeys, keys...)
	labelValues := append(keysValues[n:n], o.GetNamespace(), o.GetName(), string(o.GetUID()))
	labelValues = append(labelValues, values...)
	return &metric.Metric{LabelKeys: labelKeys, LabelValues: labelValues, Value: 1}
}

// podFamilies generates 20 families looking like those of a pod in
// kube-state-metrics: a few with many labels, a few with a series per value of
// a label, and many with a series per container.
func podFamilies(obj interface{}) []metric.FamilyInterface {
	o, err := meta.Accessor(obj)
	if err != nil {
		panic(err)
	}
	families := make([]metric.FamilyInterface, 0, 20)
	for f := range 3 {
		families = append(families, &metric.Family{
			Name: fmt.Sprintf("kube_pod_info_%d", f),
			Metrics: []*metric.Metric{podMetric(o,
				[]string{"host_ip", "pod_ip", "node", "created_by_kind", "created_by_name", "priority_class", "host_network"},
				"10.0.0.1", "10.1.0.1", "node-1", "ReplicaSet", "app-6768ddc4d", "default", "false")},
		})
	}
	for f := range 3 {
		var metrics []*metric.Metric
		for _, phase := range []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"} {
			metrics = append(metrics, podMetric(o, []string{"phase"}, phase))
		}
		families = append(families, &metric.Family{Name: fmt.Sprintf("kube_pod_status_phase_%d", f), Metrics: metrics})
	}
	for f := range 14 {
		var metrics []*metric.Metric
		for _, container := range []string{"app", "sidecar"} {
			metrics = append(metrics, podMetric(o, []string{"container"}, container))
		}
		families = append(families, &metric.Family{Name: fmt.Sprintf("kube_pod_container_%d", f), Metrics: metrics})
	}
	return families
}

// benchPods returns n pods of 50 namespaces.
func benchPods(n int) []*v1.Pod {
	pods := make([]*v1.Pod, n)
	for i := range pods {
		pods[i] = &v1.Pod{ObjectMeta: metav1.ObjectMeta{
			Namespace: fmt.Sprintf("namespace-%d", i%50),
			Name:      fmt.Sprintf("app-6768ddc4d-%05d", i),
			UID:       types.UID(fmt.Sprintf("bec19172-8abf-11ea-8546-%012d", i)),
		}}
	}
	return pods
}

// BenchmarkMetricsStoreRetainedMemory measures the memory that the store keeps
// for its series, besides the objects they come from.
func BenchmarkMetricsStoreRetainedMemory(b *testing.B) {
	pods := benchPods(2000)
	nbSeries := 0
	for _, f := range podFamilies(pods[0]) {
		f.Inspect(func(f metric.Family) { nbSeries += len(f.Metrics) })
	}
	nbSeries *= len(pods)

	b.ReportAllocs()
	var retained int64
	for b.Loop() {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)

		ms := NewMetricsStore(podFamilies, "*v1.Pod")
		for _, pod := range pods {
			if err := ms.Add(pod); err != nil {
				b.Fatal(err)
			}
		}

		runtime.GC()
		runtime.ReadMemStats(&after)
		retained = int64(after.HeapAlloc) - int64(before.HeapAlloc)
		runtime.KeepAlive(ms)
	}
	b.ReportMetric(float64(retained)/float64(nbSeries), "retained-B/series")
	b.ReportMetric(float64(nbSeries), "series")
}

// BenchmarkMetricsStorePush measures a push of all the metrics of the store, as
// the check does on every run.
func BenchmarkMetricsStorePush(b *testing.B) {
	ms := NewMetricsStore(podFamilies, "*v1.Pod")
	for _, pod := range benchPods(2000) {
		if err := ms.Add(pod); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = ms.Push(GetAllFamilies, GetAllMetrics)
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	"maps"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
)

func TestExtract(t *testing.T) {
	idsToAdd := map[string]string{"bec19172-8abf-11ea-8546-42010a80022c": "gke-charly-default-pool-6948dc89-g54n", "8b136387-8a51-11ea-8546-42010a80022c": "gke-charly-default-pool-6948dc89-4r7"}
	creationTs := int64(709655400000)
	storeName := "*v1.Node"
	metricName := "kube_node_created"

	genFunc := func(obj interface{}) []metric.FamilyInterface {
		o, err := meta.Accessor(obj)
		if err != nil {
			t.Fatal(err)
		}

		metricFamily := metric.Family{
			Name: metricName,
			Metrics: []*metric.Metric{
				{
					LabelKeys:   []string{"node"},
					LabelValues: []string{string(o.GetUID()), o.GetName()},
					Value:       float64(o.GetCreationTimestamp().Unix()),
				},
			},
		}
		return []metric.FamilyInterface{&metricFamily}
	}

	ms := NewMetricsStore(genFunc, storeName)
	for uid, name := range idsToAdd {
		s := v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				UID:               types.UID(uid),
				CreationTimestamp: metav1.Unix(creationTs, 0),
			},
		}
		err := ms.Add(&s)
		if err != nil {
			t.Fatal(err)
		}
	}
	ms.mutex.RLock()
	metrics := ms.metrics
	ms.mutex.RUnlock()

	for uid, ddMetrics := range metrics {
		for _, metricFam := range ddMetrics {
			assert.Equal(t, metricName, metricFam.Name)
			assert.Equal(t, storeName, metricFam.Type)
			for _, metric := range metricFam.ListMetrics {
				assert.Equal(t, idsToAdd[string(uid)], metric.Labels.Get("node"))
			}
		}

	}
}

func TestNewLabels(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		values   []string
		expected map[string]string
		err      error
	}{
		{
			name:     "no errors",
			keys:     []string{"bar", "ole"},
			values:   []string{"foo", "cafe"},
			expected: map[string]string{"bar": "foo", "ole": "cafe"},
		},
		{
			name:     "error",
			keys:     []string{"bar", "ole", "toolong"},
			values:   []string{"foo", "cafe"},
			expected: map[string]string{},
			err:      errLabelsLength,
		},
		{
			name:     "repeated key keeps its last value",
			keys:     []string{"bar", "ole", "bar"},
			values:   []string{"foo", "cafe", "baz"},
			expected: map[string]string{"bar": "baz", "ole": "cafe"},
		},
		{
			name:     "no labels",
			expected: map[string]string{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			labels, err := NewLabels(test.keys, test.values)
			assert.ErrorIs(t, err, test.err)
			assert.Equal(t, len(test.expected), labels.Len())
			assert.Equal(t, test.expected, maps.Collect(labels.All()))
			for k, v := range test.expected {
				value, found := labels.Lookup(k)
				assert.True(t, found)
				assert.Equal(t, v, value)
				assert.Equal(t, v, labels.Get(k))
			}
			_, found := labels.Lookup("missing")
			assert.False(t, found)
			assert.Empty(t, labels.Get("missing"))
		})
	}
}

func TestLabelsAll(t *testing.T) {
	labels, err := NewLabels([]string{"b", "a", "c"}, []string{"2", "1", "3"})
	require.NoError(t, err)

	var keys, values []string
	for k, v := range labels.All() {
		keys = append(keys, k)
		values = append(values, v)
	}
	assert.Equal(t, []string{"b", "a", "c"}, keys)
	assert.Equal(t, []string{"2", "1", "3"}, values)

	// Stopping the iteration early must not panic.
	for range labels.All() {
		break
	}
}

func TestKeysInternerKeepsRecentKeys(t *testing.T) {
	ki := keysInterner{}
	intern := func(keys ...string) []string {
		return ki.intern("kube_pod_labels", Labels{keys: keys, values: make([]string, len(keys))}).keys
	}
	a := intern("namespace", "pod", "label_app")
	b := intern("namespace", "pod", "label_team")

	// Interleaved series of two workloads keep sharing their keys
	assert.Same(t, &a[0], &intern("namespace", "pod", "label_app")[0])
	assert.Same(t, &b[0], &intern("namespace", "pod", "label_team")[0])

	// The oldest keys make room for new ones
	for i := range maxRecentKeys {
		intern("namespace", "pod", strconv.Itoa(i))
	}
	assert.NotSame(t, &a[0], &intern("namespace", "pod", "label_app")[0])
	assert.Len(t, ki, 1)
}

func TestStoreDeduplicatesKeys(t *testing.T) {
	genFunc := func(interface{}) []metric.FamilyInterface {
		return []metric.FamilyInterface{&metric.Family{
			Name: "kube_pod_labels",
			Metrics: []*metric.Metric{{
				LabelKeys:   []string{"namespace", "label_app", "label_app"},
				LabelValues: []string{"default", "first", "last"},
				Value:       1,
			}},
		}}
	}

	ms := NewMetricsStore(genFunc, "*v1.Pod")
	// Twice, as the keys of the second series aren't new to the store
	for _, uid := range []types.UID{"pod-a", "pod-b"} {
		require.NoError(t, ms.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{UID: uid}}))
		labels := ms.metrics[uid][0].ListMetrics[0].Labels
		assert.Equal(t, map[string]string{"namespace": "default", "label_app": "last"}, maps.Collect(labels.All()))
		assert.Equal(t, 2, labels.Len())
	}
}

func TestFilterMetrics(t *testing.T) {
	metrics := []DDMetric{{Val: 1}, {Val: 2}, {Val: 3}}

	all := filterMetrics(metrics, GetAllMetrics)
	assert.Same(t, &metrics[0], &all[0])
	assert.Equal(t, len(all), cap(all))

	odd := filterMetrics(metrics, func(m DDMetric) bool { return int(m.Val)%2 == 1 })
	assert.Equal(t, []DDMetric{{Val: 1}, {Val: 3}}, odd)

	assert.Empty(t, filterMetrics(metrics, func(DDMetric) bool { return false }))
}

func TestLabelsFromMap(t *testing.T) {
	labels := LabelsFromMap(map[string]string{"b": "2", "a": "1"})
	assert.Equal(t, Labels{keys: []string{"a", "b"}, values: []string{"1", "2"}}, labels)
	assert.Equal(t, 0, LabelsFromMap(nil).Len())
}

func TestStoreSharesLabelKeys(t *testing.T) {
	genFunc := func(obj interface{}) []metric.FamilyInterface {
		o, err := meta.Accessor(obj)
		require.NoError(t, err)
		return []metric.FamilyInterface{&metric.Family{
			Name: "kube_pod_info",
			Metrics: []*metric.Metric{{
				// A fresh slice for every object, as the generators of
				// kube-state-metrics return.
				LabelKeys:   []string{"namespace", "pod"},
				LabelValues: []string{o.GetNamespace(), o.GetName()},
				Value:       1,
			}},
		}}
	}

	ms := NewMetricsStore(genFunc, "*v1.Pod")
	for _, name := range []string{"pod-a", "pod-b"} {
		require.NoError(t, ms.Add(&v1.Pod{ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       types.UID(name),
		}}))
	}

	a := ms.metrics["pod-a"][0].ListMetrics[0].Labels
	b := ms.metrics["pod-b"][0].ListMetrics[0].Labels
	assert.Equal(t, "pod-a", a.Get("pod"))
	assert.Equal(t, "pod-b", b.Get("pod"))
	// Both series share the same keys.
	assert.Same(t, &a.keys[0], &b.keys[0])
}

func TestPush(t *testing.T) {
	storeName := "test"
	tests := []struct {
		name        string
		toAdd       map[types.UID][]DDMetricsFam
		familyAllow FamilyAllow
		metricAllow MetricAllow
		res         map[string][]DDMetricsFam
	}{
		{
			name: "adding single metric",
			toAdd: map[types.UID][]DDMetricsFam{
				"123": {
					{
						Type: "*v1.Nodes",
						Name: "kube_node_info",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
			},
			familyAllow: GetAllFamilies,
			metricAllow: GetAllMetrics,
			res: map[string][]DDMetricsFam{
				"kube_node_info": {
					{
						Name: "kube_node_info",
						Type: "*v1.Nodes",
						ListMetrics: []DDMetric{
							{
								Val: 1,
								Labels: LabelsFromMap(map[string]string{
									"foo": "bar",
								}),
							},
						},
					},
				},
			},
		},
		{
			name:        "no metrics",
			toAdd:       map[types.UID][]DDMetricsFam{},
			familyAllow: GetAllFamilies,
			metricAllow: GetAllMetrics,
			res:         map[string][]DDMetricsFam{},
		},
		{
			name: "complex case",
			toAdd: map[types.UID][]DDMetricsFam{
				"123": {
					{
						Type: "*v1.Nodes",
						Name: "kube_node_info",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
				"456": {
					{
						Type: "*v1.Nodes",
						Name: "kube_node_creation_ts",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"bar": "baz"}),
							},
							{
								Val:    2,
								Labels: LabelsFromMap(map[string]string{"cafe": "ole"}),
							},
						},
					},
				},
			},
			familyAllow: GetAllFamilies,
			metricAllow: GetAllMetrics,
			res: map[string][]DDMetricsFam{
				"kube_node_info": {
					{
						Name: "kube_node_info",
						Type: "*v1.Nodes",
						ListMetrics: []DDMetric{
							{
								Val: 1,
								Labels: LabelsFromMap(map[string]string{
									"foo": "bar",
								}),
							},
						},
					},
				},
				"kube_node_creation_ts": {
					{
						Name: "kube_node_creation_ts",
						Type: "*v1.Nodes",
						ListMetrics: []DDMetric{
							{
								Val: 1,
								Labels: LabelsFromMap(map[string]string{
									"bar": "baz",
								}),
							},
							{
								Val: 2,
								Labels: LabelsFromMap(map[string]string{
									"cafe": "ole",
								}),
							},
						},
					},
				},
			},
		},
		{
			name: "filter by family",
			toAdd: map[types.UID][]DDMetricsFam{
				"uid-1": {
					{
						Type: "*v1.Nodes",
						Name: "kube_node_info",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
				"uid-2": {
					{
						Type: "*v1.Pods",
						Name: "kube_pod_info",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
			},
			familyAllow: func(f DDMetricsFam) bool { return f.Name == "kube_node_info" },
			metricAllow: GetAllMetrics,
			res: map[string][]DDMetricsFam{
				"kube_node_info": {
					{
						Name: "kube_node_info",
						Type: "*v1.Nodes",
						ListMetrics: []DDMetric{
							{
								Val: 1,
								Labels: LabelsFromMap(map[string]string{
									"foo": "bar",
								}),
							},
						},
					},
				},
			},
		},
		{
			name: "filter by metric",
			toAdd: map[types.UID][]DDMetricsFam{
				"uid-1": {
					{
						Type: "*v1.Nodes",
						Name: "kube_node_info",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
				"uid-2": {
					{
						Type: "*v1.Pods",
						Name: "kube_pod_info",
						ListMetrics: []DDMetric{
							{
								Val:    2,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
			},
			familyAllow: GetAllFamilies,
			metricAllow: func(m DDMetric) bool { return m.Val == 1 },
			res: map[string][]DDMetricsFam{
				"kube_node_info": {
					{
						Name: "kube_node_info",
						Type: "*v1.Nodes",
						ListMetrics: []DDMetric{
							{
								Val: 1,
								Labels: LabelsFromMap(map[string]string{
									"foo": "bar",
								}),
							},
						},
					},
				},
				// kube_pod_info is no longer included, all metrics are filtered out
			},
		},
		{
			name: "filter by metric and family",
			toAdd: map[types.UID][]DDMetricsFam{
				"uid-1": {
					{
						Type: "*v1.Nodes",
						Name: "kube_node_info",
						ListMetrics: []DDMetric{
							{
								Val:    1,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
				"uid-2": {
					{
						Type: "*v1.Pods",
						Name: "kube_pod_info",
						ListMetrics: []DDMetric{
							{
								Val:    2,
								Labels: LabelsFromMap(map[string]string{"foo": "bar"}),
							},
						},
					},
				},
			},
			familyAllow: func(f DDMetricsFam) bool { return f.Name == "kube_node_info" },
			metricAllow: func(m DDMetric) bool { return m.Val == 1 },
			res: map[string][]DDMetricsFam{
				"kube_node_info": {
					{
						Name: "kube_node_info",
						Type: "*v1.Nodes",
						ListMetrics: []DDMetric{
							{
								Val: 1,
								Labels: LabelsFromMap(map[string]string{
									"foo": "bar",
								}),
							},
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ms := NewMetricsStore(func(interface{}) []metric.FamilyInterface { return nil }, storeName)
			ms.addMetrics(test.toAdd)
			res := ms.Push(test.familyAllow, test.metricAllow)
			assert.Equal(t, res, test.res)
		})
	}
}

func TestReplace(t *testing.T) {
	idsToAdd := map[string]string{"bec19172-8abf-11ea-8546-42010a80022c": "gke-charly-default-pool-6948dc89-g54n", "8b136387-8a51-11ea-8546-42010a80022c": "gke-charly-default-pool-6948dc89-4r7"}
	creationTs := int64(709655400000)
	storeName := "*v1.Node"
	metricName := "kube_node_created"

	genFunc := func(obj interface{}) []metric.FamilyInterface {
		o, err := meta.Accessor(obj)
		if err != nil {
			t.Fatal(err)
		}

		metricFamily := metric.Family{
			Name: metricName,
			Metrics: []*metric.Metric{
				{
					LabelKeys:   []string{"node"},
					LabelValues: []string{string(o.GetUID()), o.GetName()},
					Value:       float64(o.GetCreationTimestamp().Unix()),
				},
			},
		}
		return []metric.FamilyInterface{&metricFamily}
	}

	ms := NewMetricsStore(genFunc, storeName)
	for uid, name := range idsToAdd {
		s := v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				UID:               types.UID(uid),
				CreationTimestamp: metav1.Unix(creationTs, 0),
			},
		}
		err := ms.Add(&s)
		if err != nil {
			t.Fatal(err)
		}
	}
	ms.mutex.RLock()
	metrics := ms.metrics
	ms.mutex.RUnlock()

	// Verify that our test is setup correctly and that both metrics are in place
	require.Contains(t, metrics, types.UID("bec19172-8abf-11ea-8546-42010a80022c"))
	require.Contains(t, metrics, types.UID("8b136387-8a51-11ea-8546-42010a80022c"))

	// Create a smaller set of metrics to replace the existing metrics with. We want to make sure that metrics which
	// are not defined in the new set are removed.
	s := v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "gke-charly-default-pool-6948dc89-4r7",
			UID:               types.UID("8b136387-8a51-11ea-8546-42010a80022c"),
			CreationTimestamp: metav1.Unix(creationTs, 0),
		},
	}
	replaceList := []interface{}{&s}
	err := ms.Replace(replaceList, "")
	if err != nil {
		t.Fatal(err)
	}
	ms.mutex.RLock()
	metrics = ms.metrics
	ms.mutex.RUnlock()

	assert.NotContains(t, metrics, types.UID("bec19172-8abf-11ea-8546-42010a80022c"))
	assert.Contains(t, metrics, types.UID("8b136387-8a51-11ea-8546-42010a80022c"))
}

func TestLastStoreSyncResourceVersion(t *testing.T) {
	genFunc := func(_ interface{}) []metric.FamilyInterface { return nil }
	ms := NewMetricsStore(genFunc, "*v1.Node")
	assert.Empty(t, ms.LastStoreSyncResourceVersion())

	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{UID: "uid1", ResourceVersion: "123"}}
	require.NoError(t, ms.Add(node))
	assert.Equal(t, "123", ms.LastStoreSyncResourceVersion())

	node.ResourceVersion = "124"
	require.NoError(t, ms.Update(node))
	assert.Equal(t, "124", ms.LastStoreSyncResourceVersion())

	node.ResourceVersion = "125"
	require.NoError(t, ms.Delete(node))
	assert.Equal(t, "125", ms.LastStoreSyncResourceVersion())

	ms.Bookmark("126")
	assert.Equal(t, "126", ms.LastStoreSyncResourceVersion())

	// The resource version of the list wins over the ones of the listed objects
	other := &v1.Node{ObjectMeta: metav1.ObjectMeta{UID: "uid2", ResourceVersion: "1"}}
	require.NoError(t, ms.Replace([]interface{}{other}, "127"))
	assert.Equal(t, "127", ms.LastStoreSyncResourceVersion())
}

func (ms *MetricsStore) addMetrics(toAdd map[types.UID][]DDMetricsFam) {
	ms.mutex.Lock()
	for uid := range toAdd {
		ms.metrics[uid] = append(ms.metrics[uid], toAdd[uid]...)
	}
	ms.mutex.Unlock()
}

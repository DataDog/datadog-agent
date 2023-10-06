// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package store

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
				assert.Equal(t, idsToAdd[string(uid)], metric.Labels["node"])
			}
		}

	}
}

func TestBuildTags(t *testing.T) {
	tests := []struct {
		name     string
		in       *metric.Metric
		expected map[string]string
		err      error
	}{
		{
			name: "no errors",
			in: &metric.Metric{
				LabelValues: []string{"foo", "cafe"},
				LabelKeys:   []string{"bar", "ole"},
			},
			expected: map[string]string{
				"bar": "foo",
				"ole": "cafe",
			},
			err: nil,
		},
		{
			name: "error",
			in: &metric.Metric{
				LabelValues: []string{"foo", "cafe"},
				LabelKeys:   []string{"bar", "ole", "toolong"},
			},
			expected: map[string]string{},
			err:      fmt.Errorf("LabelKeys and LabelValues not same size"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m, err := buildTags(test.in)
			if err != nil {
				assert.Error(t, err, test.err)
			}
			assert.Len(t, m, len(test.expected))
			for k, v := range test.expected {
				assert.Equal(t, m[k], v)
			}
		})
	}
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{
									"foo": "bar",
								},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{"bar": "baz"},
							},
							{
								Val:    2,
								Labels: map[string]string{"cafe": "ole"},
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
								Labels: map[string]string{
									"foo": "bar",
								},
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
								Labels: map[string]string{
									"bar": "baz",
								},
							},
							{
								Val: 2,
								Labels: map[string]string{
									"cafe": "ole",
								},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{
									"foo": "bar",
								},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{
									"foo": "bar",
								},
							},
						},
					},
				},
				"kube_pod_info": {
					{
						Name:        "kube_pod_info",
						Type:        "*v1.Pods",
						ListMetrics: []DDMetric{},
					},
				},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{"foo": "bar"},
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
								Labels: map[string]string{
									"foo": "bar",
								},
							},
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ms := NewMetricsStore(func(i interface{}) []metric.FamilyInterface { return nil }, storeName)
			ms.addMetrics(test.toAdd)
			res := ms.Push(test.familyAllow, test.metricAllow)
			assert.Equal(t, res, test.res)
		})
	}
}

func (ms *MetricsStore) addMetrics(toAdd map[types.UID][]DDMetricsFam) {
	ms.mutex.Lock()
	for uid := range toAdd {
		ms.metrics[uid] = append(ms.metrics[uid], toAdd[uid]...)
	}
	ms.mutex.Unlock()
}

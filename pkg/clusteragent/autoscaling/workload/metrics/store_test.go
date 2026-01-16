// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
)

func TestNewPodAutoscalerMetricsStore(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)

	assert.NotNil(t, store)
	assert.NotNil(t, store.generateMetricsFunc)
}

func TestMetricsStoreAdd(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	})

	key := "test-ns/test-dpa"
	obj := &PodAutoscalerMetricsObject{
		CRD: &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-dpa",
				Namespace: "test-ns",
			},
		},
		Internal: &model.PodAutoscalerInternal{},
	}

	store.Add(key, obj)

	// Verify the metrics were stored
	var foundKey string
	store.metrics.Range(func(k, v interface{}) bool {
		foundKey = k.(string)
		metrics := v.(StructuredMetrics)
		assert.Len(t, metrics, 1)
		assert.Equal(t, "test.metric", metrics[0].Name)
		return true
	})

	assert.Equal(t, key, foundKey)
}

func TestMetricsStoreUpdate(t *testing.T) {
	callCount := 0
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		callCount++
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: float64(callCount), Tags: []string{"test:tag"}},
		}
	})

	key := "test-ns/test-dpa"
	obj := &PodAutoscalerMetricsObject{
		CRD:      &datadoghq.DatadogPodAutoscaler{},
		Internal: &model.PodAutoscalerInternal{},
	}

	// First update
	store.Update(key, obj)
	assert.Equal(t, 1, callCount)

	// Second update
	store.Update(key, obj)
	assert.Equal(t, 2, callCount)

	// Verify the metrics were updated
	val, ok := store.metrics.Load(key)
	require.True(t, ok)
	metrics := val.(StructuredMetrics)
	assert.Equal(t, 2.0, metrics[0].Value)
}

func TestMetricsStoreDelete(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"test:tag"}},
		}
	})

	key := "test-ns/test-dpa"
	obj := &PodAutoscalerMetricsObject{
		CRD:      &datadoghq.DatadogPodAutoscaler{},
		Internal: &model.PodAutoscalerInternal{},
	}

	// Add first
	store.Add(key, obj)

	// Verify it exists
	_, ok := store.metrics.Load(key)
	assert.True(t, ok)

	// Delete
	store.Delete(key, obj)

	// Verify it's gone
	_, ok = store.metrics.Load(key)
	assert.False(t, ok)
}

func TestMetricsStoreReplace(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(func(obj interface{}) StructuredMetrics {
		// Handle both CRD objects (from Replace) and PodAutoscalerMetricsObject (from Add/Update)
		var name string
		if metricObj, ok := obj.(*PodAutoscalerMetricsObject); ok {
			name = metricObj.CRD.Name
		} else if crd, ok := obj.(*datadoghq.DatadogPodAutoscaler); ok {
			name = crd.Name
		}
		return StructuredMetrics{
			{Name: "test.metric", Type: MetricTypeGauge, Value: 1.0, Tags: []string{"name:" + name}},
		}
	})

	// Add initial items
	obj1 := &PodAutoscalerMetricsObject{
		CRD: &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dpa1",
				Namespace: "test-ns",
			},
		},
		Internal: &model.PodAutoscalerInternal{},
	}
	obj2 := &PodAutoscalerMetricsObject{
		CRD: &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dpa2",
				Namespace: "test-ns",
			},
		},
		Internal: &model.PodAutoscalerInternal{},
	}

	store.Add("test-ns/dpa1", obj1)
	store.Add("test-ns/dpa2", obj2)

	// Verify initial count
	count := 0
	store.metrics.Range(func(k, v interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, 2, count)

	// Replace with new list - pass CRD objects directly since Replace uses MetaNamespaceKeyFunc
	crd3 := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dpa3",
			Namespace: "test-ns",
		},
	}

	err := store.Replace([]interface{}{crd3}, "")
	assert.NoError(t, err)

	// Verify new count
	count = 0
	store.metrics.Range(func(k, v interface{}) bool {
		count++
		return true
	})
	assert.Equal(t, 1, count)

	// Verify only dpa3 exists
	_, ok := store.metrics.Load("test-ns/dpa1")
	assert.False(t, ok)
	_, ok = store.metrics.Load("test-ns/dpa2")
	assert.False(t, ok)
	_, ok = store.metrics.Load("test-ns/dpa3")
	assert.True(t, ok)
}

func TestMetricsStoreListReturnsNil(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)
	assert.Nil(t, store.List())
}

func TestMetricsStoreListKeysReturnsNil(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)
	assert.Nil(t, store.ListKeys())
}

func TestMetricsStoreGetReturnsNil(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)
	item, exists, err := store.Get(nil)
	assert.Nil(t, item)
	assert.False(t, exists)
	assert.NoError(t, err)
}

func TestMetricsStoreGetByKeyReturnsNil(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)
	item, exists, err := store.GetByKey("some-key")
	assert.Nil(t, item)
	assert.False(t, exists)
	assert.NoError(t, err)
}

func TestMetricsStoreResyncNoOp(t *testing.T) {
	store := NewPodAutoscalerMetricsStore(GeneratePodAutoscalerMetrics)
	err := store.Resync()
	assert.NoError(t, err)
}

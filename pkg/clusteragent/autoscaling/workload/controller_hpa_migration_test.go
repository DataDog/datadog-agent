// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// newHPAIndexer builds a cache.Indexer for HPA objects with the hpaTargetIndexName index.
func newHPAIndexer(t *testing.T) cache.Indexer {
	t.Helper()
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		hpaTargetIndexName: hpaByTargetRefIndex,
	})
	return indexer
}

// addHPAToIndexer converts an HPA to unstructured and adds it to the indexer.
func addHPAToIndexer(t *testing.T, indexer cache.Indexer, hpa *autoscalingv2.HorizontalPodAutoscaler) {
	t.Helper()
	obj, err := autoscaling.ToUnstructured(hpa)
	require.NoError(t, err)
	require.NoError(t, indexer.Add(obj))
}

// newDatadogMetricIndexer builds a plain cache.Indexer suitable for DatadogMetric lookups.
func newDatadogMetricIndexer() cache.Indexer {
	return cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
}

// addDatadogMetricToIndexer converts a DatadogMetric to unstructured and adds it to the indexer.
func addDatadogMetricToIndexer(t *testing.T, indexer cache.Indexer, dm *datadoghqv1alpha1.DatadogMetric) {
	t.Helper()
	obj, err := autoscaling.ToUnstructured(dm)
	require.NoError(t, err)
	require.NoError(t, indexer.Add(obj))
}

// --- hpaHasDatadogMetricRefs ---

func TestHPAHasDatadogMetricRefs(t *testing.T) {
	makeHPA := func(metrics ...autoscalingv2.MetricSpec) *autoscalingv2.HorizontalPodAutoscaler {
		return &autoscalingv2.HorizontalPodAutoscaler{
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{Metrics: metrics},
		}
	}

	// No metrics at all
	assert.False(t, hpaHasDatadogMetricRefs(makeHPA()))

	// CPU-only HPA — should return false (no DatadogMetric needed)
	assert.False(t, hpaHasDatadogMetricRefs(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType},
		},
	})))

	// External metric without datadogmetric@ prefix
	assert.False(t, hpaHasDatadogMetricRefs(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "prometheus_requests"},
		},
	})))

	// External metric with datadogmetric@ prefix (lowercase)
	assert.True(t, hpaHasDatadogMetricRefs(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:my-metric"},
		},
	})))

	// datadogmetric@ with mixed case
	assert.True(t, hpaHasDatadogMetricRefs(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "DatadogMetric@ns:my-metric"},
		},
	})))

	// Mixed: CPU + DatadogMetric → true
	assert.True(t, hpaHasDatadogMetricRefs(makeHPA(
		autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name:   corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType},
			},
		},
		autoscalingv2.MetricSpec{
			Type: autoscalingv2.ExternalMetricSourceType,
			External: &autoscalingv2.ExternalMetricSource{
				Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:my-metric"},
			},
		},
	)))
}

// --- isCPUUsageQuery ---

func TestIsCPUUsageQuery(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"avg:container.cpu.usage{*}", true},
		{"avg:kubernetes.cpu.usage{*}", true},
		{"avg:CONTAINER.CPU.USAGE{*}", true},
		{"avg:nginx.requests{*}", false},
		{"sum:system.cpu.user{*}", false},
		{"", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isCPUUsageQuery(tt.query), "query: %q", tt.query)
	}
}

// --- validateHPAMetrics ---

func TestValidateHPAMetrics(t *testing.T) {
	makeHPA := func(metrics ...autoscalingv2.MetricSpec) *autoscalingv2.HorizontalPodAutoscaler {
		return &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "ns"},
			Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{Metrics: metrics},
		}
	}

	// Valid: pod-level CPU utilization
	assert.NoError(t, validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: pointer.Ptr(int32(50))},
		},
	})))

	// Valid: container CPU utilization
	assert.NoError(t, validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ContainerResourceMetricSourceType,
		ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
			Name:      corev1.ResourceCPU,
			Container: "app",
			Target:    autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: pointer.Ptr(int32(60))},
		},
	})))

	// Valid: pod-level CPU AverageValue (UC9 — converted at import time)
	avg := resource.MustParse("500m")
	assert.NoError(t, validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &avg},
		},
	})))

	// Valid: container CPU AverageValue (UC9)
	assert.NoError(t, validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ContainerResourceMetricSourceType,
		ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
			Name:      corev1.ResourceCPU,
			Container: "app",
			Target:    autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &avg},
		},
	})))

	// Valid: external DatadogMetric reference
	assert.NoError(t, validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:my-metric"},
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType},
		},
	})))

	// Invalid: memory resource metric
	err := validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceMemory,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType},
		},
	}))
	require.Error(t, err)
	assert.ErrorAs(t, err, new(interface {
		Reason() autoscaling.ConditionReasonType
	}))

	// Invalid: CPU with Value (cluster-wide total) target — still not supported
	err = validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.ValueMetricType},
		},
	}))
	require.Error(t, err)

	// Invalid: external metric without datadogmetric@ prefix
	err = validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "prometheus_requests"},
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType},
		},
	}))
	require.Error(t, err)
	var cr autoscaling.ConditionReason
	require.ErrorAs(t, err, &cr)
	assert.Equal(t, autoscaling.ConditionReasonUnsupportedHPAMetric, cr.Reason())

	// Invalid: pods metric type
	err = validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.PodsMetricSourceType,
	}))
	require.Error(t, err)
}

// --- findHPAForTarget ---

func TestFindHPAForTarget(t *testing.T) {
	indexer := newHPAIndexer(t)

	hpa1 := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa-1", Namespace: "ns"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "deploy-a"},
		},
	}
	hpa2 := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa-2", Namespace: "ns"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "deploy-b"},
		},
	}
	hpa3 := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa-3", Namespace: "ns"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "deploy-a"},
		},
	}
	addHPAToIndexer(t, indexer, hpa1)
	addHPAToIndexer(t, indexer, hpa2)

	// No match
	got, err := findHPAForTarget(indexer, "ns", "deploy-c")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Exact match
	got, err = findHPAForTarget(indexer, "ns", "deploy-b")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "hpa-2", got.Name)

	// Namespace isolation: different namespace should find nothing
	got, err = findHPAForTarget(indexer, "other-ns", "deploy-a")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Ambiguous: two HPAs target the same deployment
	addHPAToIndexer(t, indexer, hpa3)
	got, err = findHPAForTarget(indexer, "ns", "deploy-a")
	require.Error(t, err)
	assert.Nil(t, got)
	var cr autoscaling.ConditionReason
	require.ErrorAs(t, err, &cr)
	assert.Equal(t, autoscaling.ConditionReasonAmbiguousHPA, cr.Reason())
}

// --- resolveDatadogMetricFromCache ---

func TestResolveDatadogMetricFromCache(t *testing.T) {
	indexer := newDatadogMetricIndexer()

	dm := &datadoghqv1alpha1.DatadogMetric{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v1alpha1",
			Kind:       "DatadogMetric",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "my-metric", Namespace: "ns"},
		Spec: datadoghqv1alpha1.DatadogMetricSpec{
			Query:  "avg:container.cpu.usage{*}",
			MaxAge: metav1.Duration{Duration: 10 * time.Minute},
		},
	}
	addDatadogMetricToIndexer(t, indexer, dm)

	// Found
	spec, err := resolveDatadogMetricFromCache(indexer, "ns", "my-metric")
	require.NoError(t, err)
	require.NotNil(t, spec)
	assert.Equal(t, "avg:container.cpu.usage{*}", spec.Query)
	assert.Equal(t, 10*time.Minute, spec.MaxAge.Duration)

	// Not found
	_, err = resolveDatadogMetricFromCache(indexer, "ns", "missing-metric")
	require.Error(t, err)
	var cr autoscaling.ConditionReason
	require.ErrorAs(t, err, &cr)
	assert.Equal(t, autoscaling.ConditionReasonDatadogMetricNotFound, cr.Reason())
}

// --- extractHPAConfig ---

func TestExtractHPAConfig(t *testing.T) {
	t.Run("pod CPU utilization", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: pointer.Ptr(int32(2)),
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name:   corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: pointer.Ptr(int32(70))},
					},
				}},
			},
		}
		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), nil, nil, hpa)
		require.NoError(t, err)
		assert.Equal(t, pointer.Ptr(int32(2)), cfg.MinReplicas)
		assert.Equal(t, int32(10), cfg.MaxReplicas)
		require.NotNil(t, cfg.PodCPUUtilization)
		assert.Equal(t, int32(70), *cfg.PodCPUUtilization)
		assert.Empty(t, cfg.ContainerCPUTargets)
	})

	t.Run("container CPU utilization", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 5,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ContainerResourceMetricSourceType,
					ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
						Name:      corev1.ResourceCPU,
						Container: "app",
						Target:    autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: pointer.Ptr(int32(60))},
					},
				}},
			},
		}
		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), nil, nil, hpa)
		require.NoError(t, err)
		assert.Nil(t, cfg.PodCPUUtilization)
		require.Len(t, cfg.ContainerCPUTargets, 1)
		assert.Equal(t, "app", cfg.ContainerCPUTargets[0].ContainerName)
		assert.Equal(t, int32(60), cfg.ContainerCPUTargets[0].CPUUtilization)
	})

	t.Run("external DatadogMetric CPU usage query", func(t *testing.T) {
		ddmIndexer := newDatadogMetricIndexer()
		addDatadogMetricToIndexer(t, ddmIndexer, &datadoghqv1alpha1.DatadogMetric{
			TypeMeta:   metav1.TypeMeta{APIVersion: "datadoghq.com/v1alpha1", Kind: "DatadogMetric"},
			ObjectMeta: metav1.ObjectMeta{Name: "cpu-metric", Namespace: "ns"},
			Spec:       datadoghqv1alpha1.DatadogMetricSpec{Query: "avg:container.cpu.usage{*}"},
		})

		q := resource.MustParse("100m")
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 8,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:cpu-metric"},
						Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &q},
					},
				}},
			},
		}
		cfg, err := extractHPAConfig(context.Background(), ddmIndexer, nil, nil, hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ExternalMetrics, 1)
		em := cfg.ExternalMetrics[0]
		assert.True(t, em.IsCPUUsage)
		assert.Equal(t, "avg:container.cpu.usage{*}", em.Query)
		assert.Equal(t, defaultCustomQueryWindow, em.Window)
	})

	t.Run("external DatadogMetric custom query with MaxAge", func(t *testing.T) {
		ddmIndexer := newDatadogMetricIndexer()
		addDatadogMetricToIndexer(t, ddmIndexer, &datadoghqv1alpha1.DatadogMetric{
			TypeMeta:   metav1.TypeMeta{APIVersion: "datadoghq.com/v1alpha1", Kind: "DatadogMetric"},
			ObjectMeta: metav1.ObjectMeta{Name: "rps-metric", Namespace: "ns"},
			Spec:       datadoghqv1alpha1.DatadogMetricSpec{Query: "avg:nginx.requests{*}", MaxAge: metav1.Duration{Duration: 2 * time.Minute}},
		})

		v := resource.MustParse("500")
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 20,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:rps-metric"},
						Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &v},
					},
				}},
			},
		}
		cfg, err := extractHPAConfig(context.Background(), ddmIndexer, nil, nil, hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ExternalMetrics, 1)
		em := cfg.ExternalMetrics[0]
		assert.False(t, em.IsCPUUsage)
		assert.Equal(t, "avg:nginx.requests{*}", em.Query)
		assert.Equal(t, 2*time.Minute, em.Window)
	})

	t.Run("external DatadogMetric not found in cache", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 5,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:missing"},
						Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: func() *resource.Quantity { q := resource.MustParse("1"); return &q }()},
					},
				}},
			},
		}
		_, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), nil, nil, hpa)
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonDatadogMetricNotFound, cr.Reason())
	})

	t.Run("external DatadogMetric query with template variables is resolved", func(t *testing.T) {
		t.Setenv("TEST_CLUSTER", "prod-cluster")
		ddmIndexer := newDatadogMetricIndexer()
		addDatadogMetricToIndexer(t, ddmIndexer, &datadoghqv1alpha1.DatadogMetric{
			TypeMeta:   metav1.TypeMeta{APIVersion: "datadoghq.com/v1alpha1", Kind: "DatadogMetric"},
			ObjectMeta: metav1.ObjectMeta{Name: "rps-metric", Namespace: "ns"},
			Spec: datadoghqv1alpha1.DatadogMetricSpec{
				Query: "avg:nginx.requests{cluster:%%env_TEST_CLUSTER%%,service:web}",
			},
		})

		v := resource.MustParse("10")
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{Name: "datadogmetric@ns:rps-metric"},
						Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &v},
					},
				}},
			},
		}
		cfg, err := extractHPAConfig(context.Background(), ddmIndexer, nil, nil, hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ExternalMetrics, 1)
		// Placeholder must be resolved; the raw %%env_%% string must not reach the DPA spec.
		assert.Equal(t, "avg:nginx.requests{cluster:prod-cluster,service:web}", cfg.ExternalMetrics[0].Query)
	})
}

// --- UC9: AverageValue → Utilization conversion ---

// containerSpec is a (name, cpuRequest) pair used by buildWorkloadUnstructured.
// An empty cpuRequest produces a container with no resources.requests.cpu set,
// which is the trigger for ConditionReasonMissingCPURequest.
type containerSpec struct {
	name       string
	cpuRequest string
}

// buildWorkloadUnstructured produces an unstructured workload object (Deployment / StatefulSet /
// Rollout shape) with the given pod-template containers and optional init containers.
// Used to seed the fake dynamic client for UC9 tests.
func buildWorkloadUnstructured(t *testing.T, apiVersion, kind, namespace, name string, containers []containerSpec, initContainers []containerSpec) *unstructured.Unstructured {
	t.Helper()
	toContainerList := func(specs []containerSpec) []interface{} {
		out := make([]interface{}, 0, len(specs))
		for _, s := range specs {
			c := map[string]interface{}{"name": s.name}
			if s.cpuRequest != "" {
				c["resources"] = map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu": s.cpuRequest,
					},
				}
			}
			out = append(out, c)
		}
		return out
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"containers": toContainerList(containers),
				},
			},
		},
	}}
	if len(initContainers) > 0 {
		require.NoError(t, unstructured.SetNestedSlice(obj.Object,
			toContainerList(initContainers),
			"spec", "template", "spec", "initContainers"))
	}
	return obj
}

// newFakeDynamicClientWithDeployment seeds a fake dynamic client with a single Deployment
// carrying the provided container specs.
func newFakeDynamicClientWithDeployment(t *testing.T, ns, name string, containers []containerSpec) *dynamicfake.FakeDynamicClient {
	t.Helper()
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}:           "DeploymentList",
		{Group: "apps", Version: "v1", Resource: "statefulsets"}:          "StatefulSetList",
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"}: "RolloutList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
		buildWorkloadUnstructured(t, "apps/v1", kubernetes.DeploymentKind, ns, name, containers, nil),
	)
}

// defaultWorkloadGVRs returns the GVR table the controller would receive in production.
func defaultWorkloadGVRs() map[string]schema.GroupVersionResource {
	return map[string]schema.GroupVersionResource{
		kubernetes.DeploymentKind:  {Group: "apps", Version: "v1", Resource: "deployments"},
		kubernetes.StatefulSetKind: {Group: "apps", Version: "v1", Resource: "statefulsets"},
		kubernetes.RolloutKind:     {Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"},
	}
}

// makeAverageValueHPA returns an HPA targeting (kind, name) in namespace ns with a single
// AverageValue CPU metric of the given kind ("Resource" for pod-level, "ContainerResource"
// for per-container). containerName is only relevant for ContainerResource.
func makeAverageValueHPA(metricKind autoscalingv2.MetricSourceType, ns, kind, workloadName, containerName, avg string, minReplicas *int32, maxReplicas int32) *autoscalingv2.HorizontalPodAutoscaler {
	q := resource.MustParse(avg)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: ns},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MinReplicas: minReplicas,
			MaxReplicas: maxReplicas,
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       kind,
				Name:       workloadName,
			},
		},
	}
	switch metricKind {
	case autoscalingv2.ResourceMetricSourceType:
		hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name:   corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &q},
			},
		}}
	case autoscalingv2.ContainerResourceMetricSourceType:
		hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
			Type: autoscalingv2.ContainerResourceMetricSourceType,
			ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
				Name:      corev1.ResourceCPU,
				Container: containerName,
				Target:    autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: &q},
			},
		}}
	}
	return hpa
}

func TestExtractHPAConfig_AverageValueConversion(t *testing.T) {
	t.Run("ContainerResource CPU AverageValue → Utilization (single container)", func(t *testing.T) {
		// Container "app" requests 1000m; HPA targets 500m → 50% utilization.
		dyn := newFakeDynamicClientWithDeployment(t, "ns", "web", []containerSpec{
			{name: "app", cpuRequest: "1000m"},
		})
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "app", "500m", nil, 5)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ContainerCPUTargets, 1)
		assert.Equal(t, "app", cfg.ContainerCPUTargets[0].ContainerName)
		assert.Equal(t, int32(50), cfg.ContainerCPUTargets[0].CPUUtilization)
		assert.Nil(t, cfg.PodCPUUtilization)
	})

	t.Run("Resource CPU AverageValue → Utilization (sum of containers)", func(t *testing.T) {
		// Pod template: app=1000m + sidecar=500m = 1500m total; HPA targets 750m → 50%.
		dyn := newFakeDynamicClientWithDeployment(t, "ns", "web", []containerSpec{
			{name: "app", cpuRequest: "1000m"},
			{name: "sidecar", cpuRequest: "500m"},
		})
		hpa := makeAverageValueHPA(autoscalingv2.ResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "", "750m", nil, 5)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)
		require.NotNil(t, cfg.PodCPUUtilization)
		assert.Equal(t, int32(50), *cfg.PodCPUUtilization)
		assert.Empty(t, cfg.ContainerCPUTargets)
	})

	t.Run("ContainerResource missing requests.cpu → MissingCPURequest", func(t *testing.T) {
		dyn := newFakeDynamicClientWithDeployment(t, "ns", "web", []containerSpec{
			{name: "app"}, // no cpuRequest
		})
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "app", "500m", nil, 5)

		_, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonMissingCPURequest, cr.Reason())
	})

	t.Run("Resource all containers without requests.cpu → MissingCPURequest", func(t *testing.T) {
		dyn := newFakeDynamicClientWithDeployment(t, "ns", "web", []containerSpec{
			{name: "app"},
			{name: "sidecar"},
		})
		hpa := makeAverageValueHPA(autoscalingv2.ResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "", "500m", nil, 5)

		_, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonMissingCPURequest, cr.Reason())
	})

	t.Run("ContainerResource container not in template → MissingCPURequest", func(t *testing.T) {
		// Container "app" exists but HPA references "missing".
		dyn := newFakeDynamicClientWithDeployment(t, "ns", "web", []containerSpec{
			{name: "app", cpuRequest: "1000m"},
		})
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "missing", "500m", nil, 5)

		_, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonMissingCPURequest, cr.Reason())
	})

	t.Run("StatefulSet workload kind", func(t *testing.T) {
		scheme := runtime.NewScheme()
		listKinds := map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "statefulsets"}: "StatefulSetList",
		}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
			buildWorkloadUnstructured(t, "apps/v1", kubernetes.StatefulSetKind, "ns", "db",
				[]containerSpec{{name: "db", cpuRequest: "2"}}, nil),
		)
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", kubernetes.StatefulSetKind, "db", "db", "500m", nil, 5)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ContainerCPUTargets, 1)
		assert.Equal(t, int32(25), cfg.ContainerCPUTargets[0].CPUUtilization) // 500m / 2000m = 25%
	})

	t.Run("Argo Rollout workload kind", func(t *testing.T) {
		scheme := runtime.NewScheme()
		listKinds := map[schema.GroupVersionResource]string{
			{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"}: "RolloutList",
		}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
			buildWorkloadUnstructured(t, "argoproj.io/v1alpha1", kubernetes.RolloutKind, "ns", "canary",
				[]containerSpec{{name: "app", cpuRequest: "1"}}, nil),
		)
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", kubernetes.RolloutKind, "canary", "app", "750m", nil, 5)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ContainerCPUTargets, 1)
		assert.Equal(t, int32(75), cfg.ContainerCPUTargets[0].CPUUtilization)
	})

	t.Run("Unknown workload kind → UnsupportedHPAMetric", func(t *testing.T) {
		// targetRef.Kind = "Job" is not in workloadGVRs.
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", "Job", "batch", "worker", "500m", nil, 5)
		dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())

		_, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonUnsupportedHPAMetric, cr.Reason())
	})

	t.Run("Init containers excluded from pod-level CPU sum", func(t *testing.T) {
		// Init container with massive CPU request must NOT contribute to the pod-level sum.
		// Regular containers: 500m + 500m = 1000m; HPA targets 500m → 50%.
		scheme := runtime.NewScheme()
		listKinds := map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
			buildWorkloadUnstructured(t, "apps/v1", kubernetes.DeploymentKind, "ns", "web",
				[]containerSpec{
					{name: "app", cpuRequest: "500m"},
					{name: "sidecar", cpuRequest: "500m"},
				},
				[]containerSpec{{name: "setup", cpuRequest: "9999m"}},
			),
		)
		hpa := makeAverageValueHPA(autoscalingv2.ResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "", "500m", nil, 5)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)
		require.NotNil(t, cfg.PodCPUUtilization)
		assert.Equal(t, int32(50), *cfg.PodCPUUtilization)
	})

	t.Run("MinReplicas / MaxReplicas still imported alongside conversion", func(t *testing.T) {
		dyn := newFakeDynamicClientWithDeployment(t, "ns", "web", []containerSpec{
			{name: "app", cpuRequest: "1000m"},
		})
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "ns", kubernetes.DeploymentKind, "web", "app", "500m", pointer.Ptr(int32(3)), 12)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)
		require.NotNil(t, cfg.MinReplicas)
		assert.Equal(t, int32(3), *cfg.MinReplicas)
		assert.Equal(t, int32(12), cfg.MaxReplicas)
	})

	t.Run("end-to-end: HPA → DPA spec has constraints and converted Utilization objective", func(t *testing.T) {
		// Pipes the AverageValue HPA through both extractHPAConfig and applyHPAConfigToDPASpec
		// to confirm the final DatadogPodAutoscalerSpec carries both the replica constraints
		// (copied straight from the HPA) and the converted Utilization objective. Matches the
		// shape documented in hpa-migration.md Example 5 step 3.
		dyn := newFakeDynamicClientWithDeployment(t, "default", "api-server", []containerSpec{
			{name: "app", cpuRequest: "1000m"},
		})
		hpa := makeAverageValueHPA(autoscalingv2.ContainerResourceMetricSourceType, "default", kubernetes.DeploymentKind, "api-server", "app", "500m", pointer.Ptr(int32(2)), 10)

		cfg, err := extractHPAConfig(context.Background(), newDatadogMetricIndexer(), dyn, defaultWorkloadGVRs(), hpa)
		require.NoError(t, err)

		spec := &datadoghq.DatadogPodAutoscalerSpec{}
		applyHPAConfigToDPASpec(spec, cfg)

		// Constraints copied verbatim from the HPA.
		require.NotNil(t, spec.Constraints)
		require.NotNil(t, spec.Constraints.MinReplicas)
		assert.Equal(t, int32(2), *spec.Constraints.MinReplicas)
		require.NotNil(t, spec.Constraints.MaxReplicas)
		assert.Equal(t, int32(10), *spec.Constraints.MaxReplicas)

		// AverageValue 500m / requests.cpu 1000m → Utilization 50% on the named container.
		require.Len(t, spec.Objectives, 1)
		obj := spec.Objectives[0]
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType, obj.Type)
		require.NotNil(t, obj.ContainerResource)
		assert.Equal(t, corev1.ResourceCPU, obj.ContainerResource.Name)
		assert.Equal(t, "app", obj.ContainerResource.Container)
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType, obj.ContainerResource.Value.Type)
		require.NotNil(t, obj.ContainerResource.Value.Utilization)
		assert.Equal(t, int32(50), *obj.ContainerResource.Value.Utilization)
	})
}

// --- cpuPercentage ---

func TestCPUPercentage(t *testing.T) {
	// (target, request) → expected
	cases := []struct {
		target, request int64
		expected        int32
	}{
		{500, 1000, 50},   // half
		{1000, 1000, 100}, // exact
		{2000, 1000, 200}, // over 100%
		{1, 1000, 0},      // round-to-nearest: 0.1% → 0
		{6, 1000, 1},      // round-to-nearest: 0.6% → 1
		{999, 1000, 100},  // 99.9% → 100 (round up)
		{500, 2000, 25},   // 25%
		{750, 1000, 75},   // exact
		{1234, 1000, 123}, // round-to-nearest: 123.4% → 123
		{1235, 1000, 124}, // 123.5% → 124 (round up at .5)
	}
	for _, c := range cases {
		assert.Equal(t, c.expected, cpuPercentage(c.target, c.request),
			"cpuPercentage(%d, %d)", c.target, c.request)
	}

	// requestMilli <= 0 must not divide by zero
	assert.Equal(t, int32(0), cpuPercentage(500, 0))
	assert.Equal(t, int32(0), cpuPercentage(500, -1))
}

// --- getContainerCPURequests ---

func TestGetContainerCPURequests(t *testing.T) {
	t.Run("returns parsed quantities per container, excludes init containers", func(t *testing.T) {
		scheme := runtime.NewScheme()
		listKinds := map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds,
			buildWorkloadUnstructured(t, "apps/v1", kubernetes.DeploymentKind, "ns", "web",
				[]containerSpec{
					{name: "app", cpuRequest: "1500m"},
					{name: "sidecar", cpuRequest: "250m"},
				},
				[]containerSpec{{name: "init", cpuRequest: "9000m"}},
			),
		)

		reqs, err := getContainerCPURequests(context.Background(), dyn, defaultWorkloadGVRs(), "ns", kubernetes.DeploymentKind, "web")
		require.NoError(t, err)
		require.Len(t, reqs, 2)
		appReq := reqs["app"]
		sidecarReq := reqs["sidecar"]
		assert.Equal(t, int64(1500), appReq.MilliValue())
		assert.Equal(t, int64(250), sidecarReq.MilliValue())
		_, hasInit := reqs["init"]
		assert.False(t, hasInit, "init containers must be excluded")
	})

	t.Run("workload not found → TargetNotFound", func(t *testing.T) {
		scheme := runtime.NewScheme()
		listKinds := map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		}
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
		_, err := getContainerCPURequests(context.Background(), dyn, defaultWorkloadGVRs(), "ns", kubernetes.DeploymentKind, "missing")
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonTargetNotFound, cr.Reason())
	})

	t.Run("unknown workload kind → UnsupportedHPAMetric", func(t *testing.T) {
		dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
		_, err := getContainerCPURequests(context.Background(), dyn, defaultWorkloadGVRs(), "ns", "DaemonSet", "any")
		require.Error(t, err)
		var cr autoscaling.ConditionReason
		require.ErrorAs(t, err, &cr)
		assert.Equal(t, autoscaling.ConditionReasonUnsupportedHPAMetric, cr.Reason())
	})

	t.Run("container with malformed CPU quantity is skipped", func(t *testing.T) {
		scheme := runtime.NewScheme()
		listKinds := map[schema.GroupVersionResource]string{
			{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
		}
		obj := buildWorkloadUnstructured(t, "apps/v1", kubernetes.DeploymentKind, "ns", "web",
			[]containerSpec{{name: "app", cpuRequest: "1000m"}}, nil)
		// Tamper with the second container to have a non-parseable CPU string.
		containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		containers = append(containers, map[string]interface{}{
			"name": "broken",
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "not-a-quantity"},
			},
		})
		require.NoError(t, unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers"))
		dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, obj)

		reqs, err := getContainerCPURequests(context.Background(), dyn, defaultWorkloadGVRs(), "ns", kubernetes.DeploymentKind, "web")
		require.NoError(t, err)
		require.Len(t, reqs, 1) // only "app" — "broken" silently skipped
		appReq := reqs["app"]
		assert.Equal(t, int64(1000), appReq.MilliValue())
	})
}

// --- applyHPAConfigToDPASpec ---

func TestApplyHPAConfigToDPASpec(t *testing.T) {
	t.Run("pod CPU utilization → PodResource Utilization objective", func(t *testing.T) {
		spec := &datadoghq.DatadogPodAutoscalerSpec{}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MinReplicas:       pointer.Ptr(int32(2)),
			MaxReplicas:       10,
			PodCPUUtilization: pointer.Ptr(int32(70)),
		})
		require.NotNil(t, spec.Constraints)
		assert.Equal(t, pointer.Ptr(int32(2)), spec.Constraints.MinReplicas)
		assert.Equal(t, pointer.Ptr(int32(10)), spec.Constraints.MaxReplicas)
		require.Len(t, spec.Objectives, 1)
		obj := spec.Objectives[0]
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType, obj.Type)
		require.NotNil(t, obj.PodResource)
		assert.Equal(t, corev1.ResourceCPU, obj.PodResource.Name)
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType, obj.PodResource.Value.Type)
		assert.Equal(t, pointer.Ptr(int32(70)), obj.PodResource.Value.Utilization)
	})

	t.Run("container CPU utilization → ContainerResource Utilization objective", func(t *testing.T) {
		spec := &datadoghq.DatadogPodAutoscalerSpec{}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MaxReplicas: 5,
			ContainerCPUTargets: []ContainerCPUTarget{
				{ContainerName: "app", CPUUtilization: 60},
				{ContainerName: "sidecar", CPUUtilization: 40},
			},
		})
		require.Len(t, spec.Objectives, 2)
		for _, obj := range spec.Objectives {
			assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType, obj.Type)
			assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType, obj.ContainerResource.Value.Type)
		}
	})

	t.Run("pod CPU takes precedence over container CPU", func(t *testing.T) {
		spec := &datadoghq.DatadogPodAutoscalerSpec{}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MaxReplicas:         5,
			PodCPUUtilization:   pointer.Ptr(int32(75)),
			ContainerCPUTargets: []ContainerCPUTarget{{ContainerName: "app", CPUUtilization: 60}},
		})
		require.Len(t, spec.Objectives, 1)
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType, spec.Objectives[0].Type)
	})

	t.Run("external CPU usage → PodResource AbsoluteValue objective", func(t *testing.T) {
		q := resource.MustParse("100m")
		spec := &datadoghq.DatadogPodAutoscalerSpec{}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MaxReplicas: 8,
			ExternalMetrics: []ExternalMetricConfig{{
				Query:       "avg:container.cpu.usage{*}",
				TargetValue: &q,
				Window:      5 * time.Minute,
				IsCPUUsage:  true,
			}},
		})
		require.Len(t, spec.Objectives, 1)
		obj := spec.Objectives[0]
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType, obj.Type)
		require.NotNil(t, obj.PodResource)
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType, obj.PodResource.Value.Type)
	})

	t.Run("external custom query → CustomQuery objective", func(t *testing.T) {
		q := resource.MustParse("500")
		spec := &datadoghq.DatadogPodAutoscalerSpec{}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MaxReplicas: 20,
			ExternalMetrics: []ExternalMetricConfig{{
				Query:       "avg:nginx.requests{*}",
				TargetValue: &q,
				Window:      2 * time.Minute,
				IsCPUUsage:  false,
			}},
		})
		require.Len(t, spec.Objectives, 1)
		obj := spec.Objectives[0]
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType, obj.Type)
		require.NotNil(t, obj.CustomQuery)
		assert.Equal(t, 2*time.Minute, obj.CustomQuery.Window.Duration)
		require.Len(t, obj.CustomQuery.Request.Queries, 1)
		assert.Equal(t, "avg:nginx.requests{*}", obj.CustomQuery.Request.Queries[0].Metrics.Query)
	})

	t.Run("does not overwrite existing objectives", func(t *testing.T) {
		existing := datadoghqcommon.DatadogPodAutoscalerObjective{
			Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
		}
		spec := &datadoghq.DatadogPodAutoscalerSpec{
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{existing},
		}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MaxReplicas:       5,
			PodCPUUtilization: pointer.Ptr(int32(80)),
		})
		require.Len(t, spec.Objectives, 1)
		assert.Equal(t, existing, spec.Objectives[0])
	})

	t.Run("does not overwrite existing constraints", func(t *testing.T) {
		spec := &datadoghq.DatadogPodAutoscalerSpec{
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				MinReplicas: pointer.Ptr(int32(5)),
				MaxReplicas: pointer.Ptr(int32(20)),
			},
		}
		applyHPAConfigToDPASpec(spec, HPAConfig{
			MinReplicas: pointer.Ptr(int32(1)),
			MaxReplicas: 10,
		})
		assert.Equal(t, pointer.Ptr(int32(5)), spec.Constraints.MinReplicas)
		assert.Equal(t, pointer.Ptr(int32(20)), spec.Constraints.MaxReplicas)
	})
}

// --- hpaByTargetRefIndex ---

func TestHPAByTargetRefIndex(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa", Namespace: "ns"},
		Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "my-deploy"}},
	}
	obj, err := autoscaling.ToUnstructured(hpa)
	require.NoError(t, err)

	keys, err := hpaByTargetRefIndex(obj)
	require.NoError(t, err)
	assert.Equal(t, []string{"ns/my-deploy"}, keys)

	// Non-Unstructured input returns error
	_, err = hpaByTargetRefIndex(&autoscalingv2.HorizontalPodAutoscaler{})
	require.Error(t, err)

	// Empty scaleTargetRef name returns no keys
	empty, _ := autoscaling.ToUnstructured(&autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: "ns"},
	})
	keys, err = hpaByTargetRefIndex(empty)
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// Compile-time check that unstructured.Unstructured satisfies runtime.Object.
var _ runtime.Object = (*unstructured.Unstructured)(nil)

// model.DatadogPodAutoscalerHPAMigrationCondition is reachable from this package.
var _ = model.DatadogPodAutoscalerHPAMigrationCondition

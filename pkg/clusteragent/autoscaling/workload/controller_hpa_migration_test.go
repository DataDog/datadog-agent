// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
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
	"k8s.io/client-go/tools/cache"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
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

	// Invalid: CPU with non-Utilization target
	err = validateHPAMetrics(makeHPA(autoscalingv2.MetricSpec{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricSource{
			Name:   corev1.ResourceCPU,
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType},
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
		cfg, err := extractHPAConfig(newDatadogMetricIndexer(), hpa)
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
		cfg, err := extractHPAConfig(newDatadogMetricIndexer(), hpa)
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
		cfg, err := extractHPAConfig(ddmIndexer, hpa)
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
		cfg, err := extractHPAConfig(ddmIndexer, hpa)
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
		_, err := extractHPAConfig(newDatadogMetricIndexer(), hpa)
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
		cfg, err := extractHPAConfig(ddmIndexer, hpa)
		require.NoError(t, err)
		require.Len(t, cfg.ExternalMetrics, 1)
		// Placeholder must be resolved; the raw %%env_%% string must not reach the DPA spec.
		assert.Equal(t, "avg:nginx.requests{cluster:prod-cluster,service:web}", cfg.ExternalMetrics[0].Query)
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

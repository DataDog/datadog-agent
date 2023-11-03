// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

// This file has most of its logic copied from the KSM hpa metric family
// generators available at
// https://github.com/kubernetes/kube-state-metrics/blob/release-2.4/internal/store/horizontalpodautoscaler.go
// It exists here to provide backwards compatibility with kubernetes versions
// that use autoscaling/v2beta2, as the KSM version that we depend on uses API
// v2 instead of v2beta2.

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	autoscaling "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"

	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

type metricTargetType int

const (
	value metricTargetType = iota
	utilization
	average

	metricTargetTypeCount // Used as a length argument to arrays
)

func (m metricTargetType) String() string {
	return [...]string{"value", "utilization", "average"}[m]
}

var (
	descHorizontalPodAutoscalerAnnotationsName     = "kube_horizontalpodautoscaler_annotations"
	descHorizontalPodAutoscalerAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descHorizontalPodAutoscalerLabelsName          = "kube_horizontalpodautoscaler_labels"
	descHorizontalPodAutoscalerLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descHorizontalPodAutoscalerLabelsDefaultLabels = []string{"namespace", "horizontalpodautoscaler"}

	targetMetricLabels = []string{"metric_name", "metric_target_type"}
)

// NewHorizontalPodAutoscalerV2Beta2Factory returns a new
// HorizontalPodAutoscaler metric family generator factory.
func NewHorizontalPodAutoscalerV2Beta2Factory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &hpav2Factory{
		client: client.Cl,
	}
}

type hpav2Factory struct {
	client interface{}
}

func (f *hpav2Factory) Name() string {
	return "horizontalpodautoscalers"
}

// CreateClient is not implemented
func (f *hpav2Factory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *hpav2Factory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_info",
			"Information about this autoscaler.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				labelKeys := []string{"scaletargetref_kind", "scaletargetref_name"}
				labelValues := []string{a.Spec.ScaleTargetRef.Kind, a.Spec.ScaleTargetRef.Name}
				if a.Spec.ScaleTargetRef.APIVersion != "" {
					labelKeys = append([]string{"scaletargetref_api_version"}, labelKeys...)
					labelValues = append([]string{a.Spec.ScaleTargetRef.APIVersion}, labelValues...)
				}
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
							Value:       1,
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_metadata_generation",
			"The generation observed by the HorizontalPodAutoscaler controller.",
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.ObjectMeta.Generation),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_spec_max_replicas",
			"Upper limit for the number of pods that can be set by the autoscaler; cannot be smaller than MinReplicas.",
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.Spec.MaxReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_spec_min_replicas",
			"Lower limit for the number of pods that can be set by the autoscaler, default 1.",
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(*a.Spec.MinReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_spec_target_metric",
			"The metric specifications used by this autoscaler when calculating the desired replica count.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				ms := make([]*metric.Metric, 0, len(a.Spec.Metrics))
				for _, m := range a.Spec.Metrics {
					var metricName string

					var v [metricTargetTypeCount]int64
					var ok [metricTargetTypeCount]bool

					switch m.Type {
					case autoscaling.ObjectMetricSourceType:
						metricName = m.Object.Metric.Name

						v[value], ok[value] = m.Object.Target.Value.AsInt64()
						if m.Object.Target.AverageValue != nil {
							v[average], ok[average] = m.Object.Target.AverageValue.AsInt64()
						}
					case autoscaling.PodsMetricSourceType:
						metricName = m.Pods.Metric.Name

						v[average], ok[average] = m.Pods.Target.AverageValue.AsInt64()
					case autoscaling.ResourceMetricSourceType:
						metricName = string(m.Resource.Name)

						if ok[utilization] = (m.Resource.Target.AverageUtilization != nil); ok[utilization] {
							v[utilization] = int64(*m.Resource.Target.AverageUtilization)
						}

						if m.Resource.Target.AverageValue != nil {
							v[average], ok[average] = m.Resource.Target.AverageValue.AsInt64()
						}
					case autoscaling.ExternalMetricSourceType:
						metricName = m.External.Metric.Name

						if m.External.Target.Value != nil {
							v[value], ok[value] = m.External.Target.Value.AsInt64()
						}
						if m.External.Target.AverageValue != nil {
							v[average], ok[average] = m.External.Target.AverageValue.AsInt64()
						}
					default:
						// Skip unsupported metric type
						continue
					}

					for i := range ok {
						if ok[i] {
							ms = append(ms, &metric.Metric{
								LabelKeys:   targetMetricLabels,
								LabelValues: []string{metricName, metricTargetType(i).String()},
								Value:       float64(v[i]),
							})
						}
					}
				}
				return &metric.Family{Metrics: ms}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_status_current_replicas",
			"Current number of replicas of pods managed by this autoscaler.",
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.Status.CurrentReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_status_target_metric",
			"The current metric status used by this autoscaler when calculating the desired replica count.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				ms := make([]*metric.Metric, 0, len(a.Status.CurrentMetrics))
				for _, m := range a.Status.CurrentMetrics {
					var metricName string
					var currentMetric autoscaling.MetricValueStatus
					// The variable maps the type of metric to the corresponding value
					metricMap := make(map[metricTargetType]float64)

					switch m.Type {
					case autoscaling.ObjectMetricSourceType:
						metricName = m.Object.Metric.Name
						currentMetric = m.Object.Current
					case autoscaling.PodsMetricSourceType:
						metricName = m.Pods.Metric.Name
						currentMetric = m.Pods.Current
					case autoscaling.ResourceMetricSourceType:
						metricName = string(m.Resource.Name)
						currentMetric = m.Resource.Current
					case autoscaling.ContainerResourceMetricSourceType:
						metricName = string(m.ContainerResource.Name)
						currentMetric = m.ContainerResource.Current
					case autoscaling.ExternalMetricSourceType:
						metricName = m.External.Metric.Name
						currentMetric = m.External.Current
					default:
						// Skip unsupported metric type
						continue
					}

					if currentMetric.Value != nil {
						metricMap[value] = float64(currentMetric.Value.MilliValue()) / 1000
					}
					if currentMetric.AverageValue != nil {
						metricMap[average] = float64(currentMetric.AverageValue.MilliValue()) / 1000
					}
					if currentMetric.AverageUtilization != nil {
						metricMap[utilization] = float64(*currentMetric.AverageUtilization)
					}

					for metricTypeIndex, metricValue := range metricMap {
						ms = append(ms, &metric.Metric{
							LabelKeys:   targetMetricLabels,
							LabelValues: []string{metricName, metricTypeIndex.String()},
							Value:       metricValue,
						})
					}
				}
				return &metric.Family{Metrics: ms}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_status_desired_replicas",
			"Desired number of replicas of pods managed by this autoscaler.",
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(a.Status.DesiredReplicas),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			descHorizontalPodAutoscalerAnnotationsName,
			descHorizontalPodAutoscalerAnnotationsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				annotationKeys, annotationValues := createPrometheusLabelKeysValues("annotation", a.Annotations, allowAnnotationsList)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   annotationKeys,
							LabelValues: annotationValues,
							Value:       1,
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			descHorizontalPodAutoscalerLabelsName,
			descHorizontalPodAutoscalerLabelsHelp,
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				labelKeys, labelValues := createPrometheusLabelKeysValues("label", a.Labels, allowLabelsList)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
							Value:       1,
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_status_condition",
			"The condition of this autoscaler.",
			metric.Gauge,
			basemetrics.STABLE,
			"",
			wrapHPAFunc(func(a *autoscaling.HorizontalPodAutoscaler) *metric.Family {
				ms := make([]*metric.Metric, 0, len(a.Status.Conditions)*len(conditionStatusesV1))

				for _, c := range a.Status.Conditions {
					metrics := addConditionMetricsV1(c.Status)

					for _, m := range metrics {
						metric := m
						metric.LabelKeys = []string{"condition", "status"}
						metric.LabelValues = append([]string{string(c.Type)}, metric.LabelValues...)
						ms = append(ms, metric)
					}
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}
func (f *hpav2Factory) ExpectedType() interface{} {
	return &autoscaling.HorizontalPodAutoscaler{}
}

func (f *hpav2Factory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AutoscalingV2beta2().HorizontalPodAutoscalers(ns).Watch(context.TODO(), opts)
		},
	}
}

func wrapHPAFunc(f func(*autoscaling.HorizontalPodAutoscaler) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		hpa := obj.(*autoscaling.HorizontalPodAutoscaler)

		metricFamily := f(hpa)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descHorizontalPodAutoscalerLabelsDefaultLabels, []string{hpa.Namespace, hpa.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

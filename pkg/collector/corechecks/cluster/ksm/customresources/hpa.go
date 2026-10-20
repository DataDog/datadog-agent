// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

// This file has most of its logic copied from the KSM hpa metric family
// generators available at
// https://github.com/kubernetes/kube-state-metrics/blob/release-2.4/internal/store/horizontalpodautoscaler.go
// It exists here to provide backwards compatibility with Kubernetes < 1.23
// clusters, which serve autoscaling/v2beta2 HPAs but not autoscaling/v2 ones,
// the only version supported by the KSM version that we depend on. As the
// autoscaling/v2beta2 types and clients are gone from k8s.io/api and
// k8s.io/client-go since v0.36, these HPAs are listed and watched with the
// dynamic client and converted to autoscaling/v2, which has the same schema.
// It can be removed once support for those Kubernetes versions is officially
// dropped.

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
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

	hpav2beta2GVR = schema.GroupVersionResource{Group: "autoscaling", Version: "v2beta2", Resource: "horizontalpodautoscalers"}
)

// NewHorizontalPodAutoscalerV2Beta2Factory returns a new
// HorizontalPodAutoscaler metric family generator factory.
func NewHorizontalPodAutoscalerV2Beta2Factory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &hpav2Factory{
		client: client.DynamicInformerCl,
	}
}

type hpav2Factory struct {
	client dynamic.Interface
}

func (f *hpav2Factory) Name() string {
	return "horizontalpodautoscalers"
}

func (f *hpav2Factory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(hpav2beta2GVR), nil
}

func (f *hpav2Factory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_info",
			"Information about this autoscaler.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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
			"kube_horizontalpodautoscaler_ownerref",
			"Owner reference information about this autoscaler.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
				ownerKind, ownerName, ok := objectOwnerRef(a)
				if !ok {
					return &metric.Family{}
				}
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"ownerref_kind", "ownerref_name"},
							LabelValues: []string{ownerKind, ownerName},
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
				ms := make([]*metric.Metric, 0, len(a.Spec.Metrics))
				for _, m := range a.Spec.Metrics {
					var metricName string

					var v [metricTargetTypeCount]int64
					var ok [metricTargetTypeCount]bool

					switch m.Type {
					case autoscalingv2.ObjectMetricSourceType:
						metricName = m.Object.Metric.Name

						// An AverageValue target leaves Value unset
						if m.Object.Target.Value != nil {
							v[value], ok[value] = m.Object.Target.Value.AsInt64()
						}
						if m.Object.Target.AverageValue != nil {
							v[average], ok[average] = m.Object.Target.AverageValue.AsInt64()
						}
					case autoscalingv2.PodsMetricSourceType:
						metricName = m.Pods.Metric.Name

						if m.Pods.Target.AverageValue != nil {
							v[average], ok[average] = m.Pods.Target.AverageValue.AsInt64()
						}
					case autoscalingv2.ResourceMetricSourceType:
						metricName = string(m.Resource.Name)

						if ok[utilization] = (m.Resource.Target.AverageUtilization != nil); ok[utilization] {
							v[utilization] = int64(*m.Resource.Target.AverageUtilization)
						}

						if m.Resource.Target.AverageValue != nil {
							v[average], ok[average] = m.Resource.Target.AverageValue.AsInt64()
						}
					case autoscalingv2.ExternalMetricSourceType:
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
				ms := make([]*metric.Metric, 0, len(a.Status.CurrentMetrics))
				for _, m := range a.Status.CurrentMetrics {
					var metricName string
					var currentMetric autoscalingv2.MetricValueStatus
					// The variable maps the type of metric to the corresponding value
					metricMap := make(map[metricTargetType]float64)

					switch m.Type {
					case autoscalingv2.ObjectMetricSourceType:
						metricName = m.Object.Metric.Name
						currentMetric = m.Object.Current
					case autoscalingv2.PodsMetricSourceType:
						metricName = m.Pods.Metric.Name
						currentMetric = m.Pods.Current
					case autoscalingv2.ResourceMetricSourceType:
						metricName = string(m.Resource.Name)
						currentMetric = m.Resource.Current
					case autoscalingv2.ContainerResourceMetricSourceType:
						metricName = string(m.ContainerResource.Name)
						currentMetric = m.ContainerResource.Current
					case autoscalingv2.ExternalMetricSourceType:
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
				annotationKeys, annotationValues := kubeMapToPrometheusLabels("annotation", a.Annotations)
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
				labelKeys, labelValues := kubeMapToPrometheusLabels("label", a.Labels)
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
			wrapHPAFunc(func(a *autoscalingv2.HorizontalPodAutoscaler) *metric.Family {
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

// ExpectedType is the type handed out by ListWatch, with the autoscaling/v2beta2
// API version so that the store key derived from it (see util.GVRFromType) is
// "autoscaling/v2beta2, Resource=horizontalpodautoscalers", the key that
// manageResourcesReplacement enables instead of the KSM built-in
// "horizontalpodautoscalers" store.
func (f *hpav2Factory) ExpectedType() interface{} {
	return &autoscalingv2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: hpav2beta2GVR.GroupVersion().String(),
		},
	}
}

// ListWatch lists and watches autoscaling/v2beta2 HPAs and converts them to
// autoscaling/v2 ones.
func (f *hpav2Factory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(dynamic.NamespaceableResourceInterface).Namespace(ns)
	return &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			list, err := client.List(ctx, opts)
			if err != nil {
				return nil, err
			}

			hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.UnstructuredContent(), hpaList); err != nil {
				return nil, fmt.Errorf("cannot convert autoscaling/v2beta2 HorizontalPodAutoscalers: %w", err)
			}
			return hpaList, nil
		},
		WatchFuncWithContext: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			w, err := client.Watch(ctx, opts)
			if err != nil {
				return nil, err
			}

			return watch.Filter(w, func(event watch.Event) (watch.Event, bool) {
				// Error events carry a *metav1.Status, not an HPA
				if event.Type == watch.Error {
					return event, true
				}

				hpa, err := hpaFromV2beta2(event.Object)
				if err != nil {
					status := apierrors.NewInternalError(err).Status()
					return watch.Event{Type: watch.Error, Object: &status}, true
				}
				event.Object = hpa
				return event, true
			}), nil
		},
	}
}

func hpaFromV2beta2(obj runtime.Object) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	u, ok := obj.(runtime.Unstructured)
	if !ok {
		return nil, fmt.Errorf("unexpected object type %T", obj)
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), hpa); err != nil {
		return nil, fmt.Errorf("cannot convert autoscaling/v2beta2 HorizontalPodAutoscaler: %w", err)
	}
	return hpa, nil
}

func wrapHPAFunc(f func(*autoscalingv2.HorizontalPodAutoscaler) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		hpa := obj.(*autoscalingv2.HorizontalPodAutoscaler)

		metricFamily := f(hpa)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descHorizontalPodAutoscalerLabelsDefaultLabels, []string{hpa.Namespace, hpa.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// NewExtendedHorizontalPodAutoscalerFactory returns a factory that emits
// ownerReference-derived labels for HorizontalPodAutoscalers collected
// through the default autoscaling/v2 KSM generator. hpav2Factory above only
// runs as a backwards-compat replacement on clusters that don't serve
// autoscaling/v2 (see manageResourcesReplacement); this factory covers the
// complementary, and far more common, case where the upstream KSM generator
// handles autoscaling/v2 directly and therefore never sees this
// enrichment.
func NewExtendedHorizontalPodAutoscalerFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &extendedHPAFactory{
		client: client.InformerCl,
	}
}

type extendedHPAFactory struct {
	client kubernetes.Interface
}

func (f *extendedHPAFactory) Name() string {
	return "horizontalpodautoscalers_extended"
}

func (f *extendedHPAFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *extendedHPAFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			"kube_horizontalpodautoscaler_ownerref",
			"Owner reference information about this autoscaler.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			func(obj interface{}) *metric.Family {
				hpa := obj.(*autoscalingv2.HorizontalPodAutoscaler)

				ownerKind, ownerName, ok := objectOwnerRef(hpa)
				if !ok {
					return &metric.Family{}
				}

				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   []string{"namespace", "horizontalpodautoscaler", "ownerref_kind", "ownerref_name"},
							LabelValues: []string{hpa.Namespace, hpa.Name, ownerKind, ownerName},
							Value:       1,
						},
					},
				}
			},
		),
	}
}

func (f *extendedHPAFactory) ExpectedType() interface{} {
	return &autoscalingv2.HorizontalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HorizontalPodAutoscaler",
			APIVersion: autoscalingv2.SchemeGroupVersion.String(),
		},
	}
}

func (f *extendedHPAFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, opts)
		},
		WatchFuncWithContext: func(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.AutoscalingV2().HorizontalPodAutoscalers(ns).Watch(ctx, opts)
		},
	}
}

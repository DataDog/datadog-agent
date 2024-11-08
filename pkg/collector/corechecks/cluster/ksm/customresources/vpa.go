// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

// This file has most of its logic copied from the KSM vpa metric family
// generators available at
// https://github.com/kubernetes/kube-state-metrics/blob/release-2.8/internal/store/verticalpodautoscaler.go
// It exists here to provide backwards compatibility because the support of VPA has been dropped upstream by:
// https://github.com/kubernetes/kube-state-metrics/pull/2017

import (
	"context"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/constant"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

var (
	descVerticalPodAutoscalerAnnotationsName     = "kube_verticalpodautoscaler_annotations"
	descVerticalPodAutoscalerAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descVerticalPodAutoscalerLabelsName          = "kube_verticalpodautoscaler_labels"
	descVerticalPodAutoscalerLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descVerticalPodAutoscalerLabelsDefaultLabels = []string{"namespace", "verticalpodautoscaler", "target_api_version", "target_kind", "target_name"}
)

// NewVerticalPodAutoscalerFactory returns a new VerticalPodAutoscalers metric family generator factory.
func NewVerticalPodAutoscalerFactory(client *dynamic.DynamicClient) customresource.RegistryFactory {
	return &vpaFactory{
		client: client,
	}
}

type vpaFactory struct {
	client *dynamic.DynamicClient
}

func (f *vpaFactory) Name() string {
	return "verticalpodautoscalers"
}

func (f *vpaFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(schema.GroupVersionResource{
		Group:    v1.SchemeGroupVersion.Group,
		Version:  v1.SchemeGroupVersion.Version,
		Resource: "verticalpodautoscalers",
	}), nil
}

func (f *vpaFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			descVerticalPodAutoscalerAnnotationsName,
			descVerticalPodAutoscalerAnnotationsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
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
			descVerticalPodAutoscalerLabelsName,
			descVerticalPodAutoscalerLabelsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
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
			"kube_verticalpodautoscaler_spec_updatepolicy_updatemode",
			"Update mode of the VerticalPodAutoscaler.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}

				if a.Spec.UpdatePolicy == nil || a.Spec.UpdatePolicy.UpdateMode == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}

				for _, mode := range []v1.UpdateMode{
					v1.UpdateModeOff,
					v1.UpdateModeInitial,
					v1.UpdateModeRecreate,
					v1.UpdateModeAuto,
				} {
					var v float64
					if *a.Spec.UpdatePolicy.UpdateMode == mode {
						v = 1
					} else {
						v = 0
					}
					ms = append(ms, &metric.Metric{
						LabelKeys:   []string{"update_mode"},
						LabelValues: []string{string(mode)},
						Value:       v,
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_verticalpodautoscaler_spec_resourcepolicy_container_policies_minallowed",
			"Minimum resources the VerticalPodAutoscaler can set for containers matching the name.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Spec.ResourcePolicy == nil || a.Spec.ResourcePolicy.ContainerPolicies == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}

				for _, c := range a.Spec.ResourcePolicy.ContainerPolicies {
					ms = append(ms, vpaResourcesToMetrics(c.ContainerName, c.MinAllowed)...)

				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_verticalpodautoscaler_spec_resourcepolicy_container_policies_maxallowed",
			"Maximum resources the VerticalPodAutoscaler can set for containers matching the name.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Spec.ResourcePolicy == nil || a.Spec.ResourcePolicy.ContainerPolicies == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}

				for _, c := range a.Spec.ResourcePolicy.ContainerPolicies {
					ms = append(ms, vpaResourcesToMetrics(c.ContainerName, c.MaxAllowed)...)
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_lowerbound",
			"Minimum resources the container can use before the VerticalPodAutoscaler updater evicts it.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}

				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					ms = append(ms, vpaResourcesToMetrics(c.ContainerName, c.LowerBound)...)
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_upperbound",
			"Maximum resources the container can use before the VerticalPodAutoscaler updater evicts it.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}

				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					ms = append(ms, vpaResourcesToMetrics(c.ContainerName, c.UpperBound)...)
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_target",
			"Target resources the VerticalPodAutoscaler recommends for the container.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					ms = append(ms, vpaResourcesToMetrics(c.ContainerName, c.Target)...)
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_verticalpodautoscaler_status_recommendation_containerrecommendations_uncappedtarget",
			"Target resources the VerticalPodAutoscaler recommends for the container ignoring bounds.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapVPAFunc(func(a *v1.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					ms = append(ms, vpaResourcesToMetrics(c.ContainerName, c.UncappedTarget)...)
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
	}
}

func (f *vpaFactory) ExpectedType() interface{} {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("VerticalPodAutoscaler"))
	return &u
}

func vpaResourcesToMetrics(containerName string, resources corev1.ResourceList) []*metric.Metric {
	ms := []*metric.Metric{}
	for resourceName, val := range resources {
		switch resourceName {
		case corev1.ResourceCPU:
			ms = append(ms, &metric.Metric{
				LabelValues: []string{containerName, sanitizeLabelName(string(resourceName)), string(constant.UnitCore)},
				Value:       float64(val.MilliValue()) / 1000,
			})
		case corev1.ResourceStorage:
			fallthrough
		case corev1.ResourceEphemeralStorage:
			fallthrough
		case corev1.ResourceMemory:
			ms = append(ms, &metric.Metric{
				LabelValues: []string{containerName, sanitizeLabelName(string(resourceName)), string(constant.UnitByte)},
				Value:       float64(val.Value()),
			})
		}
	}
	for _, metric := range ms {
		metric.LabelKeys = []string{"container", "resource", "unit"}
	}
	return ms
}

func wrapVPAFunc(f func(*v1.VerticalPodAutoscaler) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		vpa := obj.(*v1.VerticalPodAutoscaler)

		metricFamily := f(vpa)
		targetRef := vpa.Spec.TargetRef

		// targetRef was not a mandatory field, which can lead to a nil pointer exception here.
		// However, we still want to expose metrics to be able:
		// * to alert about VPA objects without target refs
		// * to count the right amount of VPA objects in a cluster
		if targetRef == nil {
			targetRef = &autoscalingv1.CrossVersionObjectReference{}
		}

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descVerticalPodAutoscalerLabelsDefaultLabels, []string{vpa.Namespace, vpa.Name, targetRef.APIVersion, targetRef.Kind, targetRef.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

func (f *vpaFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(dynamic.NamespaceableResourceInterface).Namespace(ns)
	ctx := context.Background()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.Watch(ctx, opts)
		},
	}
}

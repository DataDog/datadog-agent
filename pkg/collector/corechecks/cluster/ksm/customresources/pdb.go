// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

// This file has most of its logic copied from the KSM pdb metric family
// generators available at
// https://github.com/kubernetes/kube-state-metrics/blob/release-2.4/internal/store/poddisruptionbudget.go
// It exists here to provide backwards compatibility with k8s <1.19, as KSM 2.4
// uses API v1 instead of v1beta1: https://github.com/kubernetes/kube-state-metrics/pull/1491

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/component-base/metrics"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

var (
	descPodDisruptionBudgetLabelsDefaultLabels = []string{"namespace", "poddisruptionbudget"}
	descPodDisruptionBudgetAnnotationsName     = "kube_poddisruptionbudget_annotations"
	descPodDisruptionBudgetAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descPodDisruptionBudgetLabelsName          = "kube_poddisruptionbudget_labels"
	descPodDisruptionBudgetLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
)

// NewPodDisruptionBudgetV1Beta1Factory returns a new PodDisruptionBudgets metric family generator factory.
func NewPodDisruptionBudgetV1Beta1Factory(client *dynamic.DynamicClient) customresource.RegistryFactory {
	return &pdbv1beta1Factory{
		client: client,
	}
}

type pdbv1beta1Factory struct {
	client *dynamic.DynamicClient
}

func (f *pdbv1beta1Factory) Name() string {
	return "poddisruptionbudgets"
}

func (f *pdbv1beta1Factory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(schema.GroupVersionResource{
		Group:    policyv1beta1.GroupName,
		Version:  policyv1beta1.SchemeGroupVersion.Version,
		Resource: "poddisruptionbudgets",
	}), nil
}

func (f *pdbv1beta1Factory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			descPodDisruptionBudgetAnnotationsName,
			descPodDisruptionBudgetAnnotationsHelp,
			metric.Gauge,
			metrics.ALPHA,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				annotationKeys, annotationValues := kubeMapToPrometheusLabels("annotation", p.Annotations)
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
			descPodDisruptionBudgetLabelsName,
			descPodDisruptionBudgetLabelsHelp,
			metric.Gauge,
			metrics.ALPHA,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				labelKeys, labelValues := kubeMapToPrometheusLabels("label", p.Labels)
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
			"kube_poddisruptionbudget_created",
			"Unix creation timestamp",
			metric.Gauge,
			metrics.STABLE,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				ms := []*metric.Metric{}

				if !p.CreationTimestamp.IsZero() {
					ms = append(ms, &metric.Metric{
						Value: float64(p.CreationTimestamp.Unix()),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_poddisruptionbudget_status_current_healthy",
			"Current number of healthy pods",
			metric.Gauge,
			metrics.STABLE,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.CurrentHealthy),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_poddisruptionbudget_status_desired_healthy",
			"Minimum desired number of healthy pods",
			metric.Gauge,
			metrics.STABLE,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.DesiredHealthy),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_poddisruptionbudget_status_pod_disruptions_allowed",
			"Number of pod disruptions that are currently allowed",
			metric.Gauge,
			metrics.STABLE,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.DisruptionsAllowed),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_poddisruptionbudget_status_expected_pods",
			"Total number of pods counted by this disruption budget",
			metric.Gauge,
			metrics.STABLE,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.ExpectedPods),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGeneratorWithStability(
			"kube_poddisruptionbudget_status_observed_generation",
			"Most recent generation observed when updating this PDB status",
			metric.Gauge,
			metrics.STABLE,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.ObservedGeneration),
						},
					},
				}
			}),
		),
	}
}

func (f *pdbv1beta1Factory) ExpectedType() interface{} {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(policyv1beta1.SchemeGroupVersion.WithKind("PodDisruptionBudget"))
	return &u
}

func (f *pdbv1beta1Factory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
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

func wrapPodDisruptionBudgetFunc(f func(*policyv1beta1.PodDisruptionBudget) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		podDisruptionBudget := &policyv1beta1.PodDisruptionBudget{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).Object, podDisruptionBudget); err != nil {
			log.Warnf("cannot decode object %q into policyv1beta1.PodDisruptionBudget, err=%s, skipping", obj.(*unstructured.Unstructured).Object["apiVersion"], err)
			return nil
		}

		metricFamily := f(podDisruptionBudget)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descPodDisruptionBudgetLabelsDefaultLabels, []string{podDisruptionBudget.Namespace, podDisruptionBudget.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

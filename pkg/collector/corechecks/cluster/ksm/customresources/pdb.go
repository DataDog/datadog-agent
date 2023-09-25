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

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/component-base/metrics"
	basemetrics "k8s.io/component-base/metrics"

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
func NewPodDisruptionBudgetV1Beta1Factory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &pdbv1beta1Factory{
		client: client.Cl,
	}
}

type pdbv1beta1Factory struct {
	client interface{}
}

func (f *pdbv1beta1Factory) Name() string {
	return "poddisruptionbudgets"
}

// CreateClient is not implemented
func (f *pdbv1beta1Factory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *pdbv1beta1Factory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			descPodDisruptionBudgetAnnotationsName,
			descPodDisruptionBudgetAnnotationsHelp,
			metric.Gauge,
			metrics.ALPHA,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				annotationKeys, annotationValues := createPrometheusLabelKeysValues("annotation", p.Annotations, allowAnnotationsList)
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
			basemetrics.ALPHA,
			"",
			wrapPodDisruptionBudgetFunc(func(p *policyv1beta1.PodDisruptionBudget) *metric.Family {
				labelKeys, labelValues := createPrometheusLabelKeysValues("label", p.Labels, allowLabelsList)
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
			basemetrics.STABLE,
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
			basemetrics.STABLE,
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
			basemetrics.STABLE,
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
			basemetrics.STABLE,
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
			basemetrics.STABLE,
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
			basemetrics.STABLE,
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
	return &policyv1beta1.PodDisruptionBudget{}
}

func (f *pdbv1beta1Factory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(kubernetes.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.PolicyV1beta1().PodDisruptionBudgets(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.PolicyV1beta1().PodDisruptionBudgets(ns).Watch(context.TODO(), opts)
		},
	}
}

func wrapPodDisruptionBudgetFunc(f func(*policyv1beta1.PodDisruptionBudget) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		podDisruptionBudget := obj.(*policyv1beta1.PodDisruptionBudget)

		metricFamily := f(podDisruptionBudget)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descPodDisruptionBudgetLabelsDefaultLabels, []string{podDisruptionBudget.Namespace, podDisruptionBudget.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

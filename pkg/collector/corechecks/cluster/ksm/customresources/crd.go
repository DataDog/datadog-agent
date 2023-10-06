// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	crd "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"

	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

var (
	descCustomResourceDefinitionAnnotationsName     = "kube_customresourcedefinition_annotations"
	descCustomResourceDefinitionAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descCustomResourceDefinitionLabelsName          = "kube_customresourcedefinition_labels"
	descCustomResourceDefinitionLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descCustomResourceDefinitionLabelsDefaultLabels = []string{"customresourcedefinition"}
)

// NewCustomResourceDefinitionFactory returns a new CustomResourceDefinition
// metric family generator factory.
func NewCustomResourceDefinitionFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &crdFactory{
		client: client.CRDClient,
	}
}

type crdFactory struct {
	client interface{}
}

func (f *crdFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			descCustomResourceDefinitionAnnotationsName,
			descCustomResourceDefinitionAnnotationsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapCustomResourceDefinition(func(c *crd.CustomResourceDefinition) *metric.Family {
				annotationKeys, annotationValues := createPrometheusLabelKeysValues("annotation", c.Annotations, allowAnnotationsList)
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
			descCustomResourceDefinitionLabelsName,
			descCustomResourceDefinitionLabelsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapCustomResourceDefinition(func(c *crd.CustomResourceDefinition) *metric.Family {
				labelKeys, labelValues := createPrometheusLabelKeysValues("label", c.Labels, allowLabelsList)
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
			"kube_customresourcedefinition_status_condition",
			"The condition of this custom resource definition.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapCustomResourceDefinition(func(c *crd.CustomResourceDefinition) *metric.Family {
				ms := make([]*metric.Metric, 0, len(c.Status.Conditions)*len(conditionStatusesExtensionV1))

				for _, c := range c.Status.Conditions {
					metrics := addConditionMetricsExtensionV1(c.Status)

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

func (f *crdFactory) Name() string {
	return "customresourcedefinitions"
}

// CreateClient is not implemented
func (f *crdFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *crdFactory) ExpectedType() interface{} {
	return &crd.CustomResourceDefinition{}
}

func (f *crdFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(clientset.Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.ApiextensionsV1().CustomResourceDefinitions().List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.ApiextensionsV1().CustomResourceDefinitions().Watch(context.TODO(), opts)
		},
	}
}

func wrapCustomResourceDefinition(f func(*v1.CustomResourceDefinition) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		crd := obj.(*crd.CustomResourceDefinition)

		metricFamily := f(crd)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descCustomResourceDefinitionLabelsDefaultLabels, []string{crd.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package customresources

import (
	"context"

	crd "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"

	crdClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
)

var (
	descCustomResourceDefinitionAnnotationsName     = "kube_customresourcedefinition_annotations"
	descCustomResourceDefinitionAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descCustomResourceDefinitionLabelsName          = "kube_customresourcedefinition_labels"
	descCustomResourceDefinitionLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descCustomResourceDefinitionLabelsDefaultLabels = []string{"namespace", "customresourcedefinition"}
)

// NewCustomResourceDefinitionFactory returns a new CustomResourceDefinition
// metric family generator factory.
func NewCustomResourceDefinitionFactory() customresource.RegistryFactory {
	return &crdFactory{}
}

type crdFactory struct{}

func (f *crdFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			descCustomResourceDefinitionAnnotationsName,
			descCustomResourceDefinitionAnnotationsHelp,
			metric.Gauge,
			"",
			wrapCustomResourceDefinition(func(p *crd.CustomResourceDefinition) *metric.Family {
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
		*generator.NewFamilyGenerator(
			descCustomResourceDefinitionLabelsName,
			descCustomResourceDefinitionLabelsHelp,
			metric.Gauge,
			"",
			wrapCustomResourceDefinition(func(p *crd.CustomResourceDefinition) *metric.Family {
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
	}
}

func (f *crdFactory) Name() string {
	return "customresourcedefinition"
}

// CreateClient is not implemented
func (f *crdFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	panic("not implemented")
}

func (f *crdFactory) ExpectedType() interface{} {
	return &crd.CustomResourceDefinition{}
}

func (f *crdFactory) ListWatch(customResourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(crdClient.ApiextensionsV1Client)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return client.CustomResourceDefinitions().List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return client.CustomResourceDefinitions().Watch(context.TODO(), opts)
		},
	}
}

func wrapCustomResourceDefinition(f func(*v1.CustomResourceDefinition) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		crd := obj.(*crd.CustomResourceDefinition)

		metricFamily := f(crd)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descPodDisruptionBudgetLabelsDefaultLabels, []string{crd.Namespace, crd.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

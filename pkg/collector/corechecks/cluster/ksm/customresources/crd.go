// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"

	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
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
func NewCustomResourceDefinitionFactory(client *dynamic.DynamicClient) customresource.RegistryFactory {
	return &crdFactory{
		client: client,
	}
}

type crdFactory struct {
	client *dynamic.DynamicClient
}

func (f *crdFactory) MetricFamilyGenerators() []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			descCustomResourceDefinitionAnnotationsName,
			descCustomResourceDefinitionAnnotationsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapCustomResourceDefinition(func(c *v1.CustomResourceDefinition) *metric.Family {
				annotationKeys, annotationValues := kubeMapToPrometheusLabels("annotation", c.Annotations)
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
			wrapCustomResourceDefinition(func(c *v1.CustomResourceDefinition) *metric.Family {
				labelKeys, labelValues := kubeMapToPrometheusLabels("label", c.Labels)
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
			wrapCustomResourceDefinition(func(c *v1.CustomResourceDefinition) *metric.Family {
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

func (f *crdFactory) CreateClient(_ *rest.Config) (interface{}, error) {
	return f.client.Resource(schema.GroupVersionResource{
		Group:    v1.GroupName,
		Version:  v1.SchemeGroupVersion.Version,
		Resource: "customresourcedefinitions",
	}), nil
}

func (f *crdFactory) ExpectedType() interface{} {
	u := unstructured.Unstructured{}
	u.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("CustomResourceDefinition"))
	return &u
}

func (f *crdFactory) ListWatch(customResourceClient interface{}, _ string, fieldSelector string) cache.ListerWatcher {
	client := customResourceClient.(dynamic.ResourceInterface)
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

func wrapCustomResourceDefinition(f func(*v1.CustomResourceDefinition) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		crd := &v1.CustomResourceDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.(*unstructured.Unstructured).Object, crd); err != nil {
			log.Warnf("cannot decode object %q into v1.CustomResourceDefinition, err=%s, skipping", obj.(*unstructured.Unstructured).Object["apiVersion"], err)
			return nil
		}

		metricFamily := f(crd)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descCustomResourceDefinitionLabelsDefaultLabels, []string{crd.Name}, m.LabelKeys, m.LabelValues)
		}

		return metricFamily
	}
}

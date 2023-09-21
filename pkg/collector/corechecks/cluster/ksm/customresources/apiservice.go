// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	basemetrics "k8s.io/component-base/metrics"
	v1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	apiregistrationclient "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

var (
	descAPIServiceAnnotationsName     = "kube_apiservice_annotations"
	descAPIServiceAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descAPIServiceLabelsName          = "kube_apiservice_labels"
	descAPIServiceLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descAPIServiceLabelsDefaultLabels = []string{"apiservice"}
)

// NewAPIServiceFactory returns a new APIService metric family generator factory.
func NewAPIServiceFactory(client *apiserver.APIClient) customresource.RegistryFactory {
	return &apiserviceFactory{
		client: client.APISClient,
	}
}

type apiserviceFactory struct {
	client interface{}
}

func (f *apiserviceFactory) CreateClient(cfg *rest.Config) (interface{}, error) {
	return f.client, nil
}

func (f *apiserviceFactory) Name() string {
	return "apiservices"
}

func (f *apiserviceFactory) MetricFamilyGenerators(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGeneratorWithStability(
			descAPIServiceAnnotationsName,
			descAPIServiceAnnotationsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapAPIServiceFunc(func(a *v1.APIService) *metric.Family {
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
			descAPIServiceLabelsName,
			descAPIServiceLabelsHelp,
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapAPIServiceFunc(func(a *v1.APIService) *metric.Family {
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
			"kube_apiservice_status_condition",
			"The condition of this APIService.",
			metric.Gauge,
			basemetrics.ALPHA,
			"",
			wrapAPIServiceFunc(func(a *v1.APIService) *metric.Family {
				ms := make([]*metric.Metric, 0, len(a.Status.Conditions)*len(conditionStatusesAPIServicesV1))

				for _, c := range a.Status.Conditions {
					metrics := addConditionMetricsAPIServicesV1(c.Status)

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

func (f *apiserviceFactory) ExpectedType() interface{} {
	return &v1.APIService{}
}

func (f *apiserviceFactory) ListWatch(customresourceClient interface{}, ns string, fieldSelector string) cache.ListerWatcher {
	client := customresourceClient.(apiregistrationclient.ApiregistrationV1Interface)
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			return client.APIServices().List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			return client.APIServices().Watch(context.TODO(), opts)
		},
	}
}

func wrapAPIServiceFunc(f func(*v1.APIService) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		apiservice := obj.(*v1.APIService)

		metricFamily := f(apiservice)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys, m.LabelValues = mergeKeyValues(descAPIServiceLabelsDefaultLabels, []string{apiservice.Name}, m.LabelKeys, m.LabelValues)
		}
		return metricFamily
	}
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type externalMetric struct {
	info  provider.ExternalMetricInfo
	value external_metrics.ExternalMetricValue
}

type datadogProvider struct {
	client dynamic.ClientPool
	mapper apimeta.RESTMapper

	values          map[provider.CustomMetricInfo]int64
	externalMetrics []externalMetric
	resVersion      string
	store           Store
}

// NewDatadogProvider creates a Custom Metrics and External Metrics Provider.
func NewDatadogProvider(client dynamic.ClientPool, mapper apimeta.RESTMapper, store Store) provider.MetricsProvider {
	return &datadogProvider{
		client: client,
		mapper: mapper,
		values: make(map[provider.CustomMetricInfo]int64),
		store:  store,
	}
}

// GetRootScopedMetricByName - Not implemented
func (p *datadogProvider) GetRootScopedMetricByName(groupResource schema.GroupResource, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("not Implemented - GetRootScopedMetricByName")
}

// GetRootScopedMetricBySelector - Not implemented
func (p *datadogProvider) GetRootScopedMetricBySelector(groupResource schema.GroupResource, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("not Implemented - GetRootScopedMetricBySelector")
}

// GetNamespacedMetricByName - Not implemented
func (p *datadogProvider) GetNamespacedMetricByName(groupResource schema.GroupResource, namespace string, name string, metricName string) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("not Implemented - GetNamespacedMetricByName")
}

// GetNamespacedMetricBySelector - Not implemented
func (p *datadogProvider) GetNamespacedMetricBySelector(groupResource schema.GroupResource, namespace string, selector labels.Selector, metricName string) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("not Implemented - GetNamespacedMetricBySelector")
}

// ListAllMetrics reads from a ConfigMap, similarly to ListExternalMetrics
// TODO implement the in cluster Custom Metrics Provider to use the ListAllMetrics
func (p *datadogProvider) ListAllMetrics() []provider.CustomMetricInfo {
	return nil
}

// ListAllExternalMetrics is called every 30 seconds, although this is configurable on the API Server's end.
func (p *datadogProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	var externalMetricsInfoList []provider.ExternalMetricInfo
	var externalMetricsList []externalMetric

	rawMetrics, err := p.store.ListAllExternalMetricValues()
	if err != nil {
		log.Errorf("Could not list the external metrics in the store: %s", err.Error())
		return externalMetricsInfoList
	}

	for _, metric := range rawMetrics {
		// Only metrics that exist in Datadog and available are eligible to be evaluated in the HPA process.
		if !metric.Valid {
			continue
		}
		var extMetric externalMetric
		extMetric.info = provider.ExternalMetricInfo{
			Metric: metric.MetricName,
			Labels: metric.Labels,
		}
		extMetric.value = external_metrics.ExternalMetricValue{
			MetricName:   metric.MetricName,
			MetricLabels: metric.Labels,
			Value:        *resource.NewQuantity(metric.Value, resource.DecimalSI),
		}
		externalMetricsList = append(externalMetricsList, extMetric)

		externalMetricsInfoList = append(externalMetricsInfoList, provider.ExternalMetricInfo{
			Metric: metric.MetricName,
			Labels: metric.Labels,
		})
	}
	p.externalMetrics = externalMetricsList
	log.Debugf("ListAllExternalMetrics returns %d metrics", len(externalMetricsInfoList))
	return externalMetricsInfoList
}

// GetExternalMetric is called every 30 seconds as a result of:
// - The registering of the External Metrics Provider
// - The creation of a HPA manifest with an External metrics type.
// - The validation of the metrics against Datadog
func (p *datadogProvider) GetExternalMetric(namespace string, metricName string, metricSelector labels.Selector) (*external_metrics.ExternalMetricValueList, error) {
	matchingMetrics := []external_metrics.ExternalMetricValue{}

	for _, metric := range p.externalMetrics {
		metricFromDatadog := external_metrics.ExternalMetricValue{
			MetricName:   metricName,
			MetricLabels: metric.info.Labels,
			Value:        metric.value.Value,
		}
		if metric.info.Metric == metricName &&
			metricSelector.Matches(labels.Set(metric.info.Labels)) {
			metricValue := metricFromDatadog
			metricValue.Timestamp = metav1.Now()
			matchingMetrics = append(matchingMetrics, metricValue)
		}
	}
	log.Tracef("External metrics returned: %#v", matchingMetrics)
	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

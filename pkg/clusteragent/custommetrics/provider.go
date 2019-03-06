// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"strings"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/metrics/pkg/apis/custom_metrics"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type externalMetric struct {
	info  provider.ExternalMetricInfo
	value external_metrics.ExternalMetricValue
}

type datadogProvider struct {
	client dynamic.Interface
	mapper apimeta.RESTMapper

	values          map[provider.CustomMetricInfo]int64
	externalMetrics []externalMetric
	resVersion      string
	store           Store
	timestamp       int64
	maxAge          int64
}

// NewDatadogProvider creates a Custom Metrics and External Metrics Provider.
func NewDatadogProvider(client dynamic.Interface, mapper apimeta.RESTMapper, store Store) provider.MetricsProvider {
	maxAge := config.Datadog.GetInt64("external_metrics_provider.local_copy_refresh_rate")
	return &datadogProvider{
		client: client,
		mapper: mapper,
		values: make(map[provider.CustomMetricInfo]int64),
		store:  store,
		maxAge: maxAge,
	}
}

// GetMetricByName - Not implemented
func (p *datadogProvider) GetMetricByName(name types.NamespacedName, info provider.CustomMetricInfo) (*custom_metrics.MetricValue, error) {
	return nil, fmt.Errorf("not Implemented - GetMetricByName")
}

// GetMetricBySelector - Not implemented
func (p *datadogProvider) GetMetricBySelector(namespace string, selector labels.Selector, info provider.CustomMetricInfo) (*custom_metrics.MetricValueList, error) {
	return nil, fmt.Errorf("not Implemented - GetMetricBySelector")
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

	copyAge := metav1.Now().Unix() - p.timestamp
	if copyAge < p.maxAge {
		log.Tracef("Local copy is recent enough, not querying the GlobalStore. Remaining %d seconds before next sync", p.maxAge-copyAge)
		for _, in := range p.externalMetrics {
			externalMetricsInfoList = append(externalMetricsInfoList, in.info)
		}
		return externalMetricsInfoList
	}

	log.Debugf("Local copy of external metrics from the global store is outdated by %d seconds, resyncing now", copyAge-p.maxAge)
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
		}
		// Avoid overflowing when trying to get a 10^3 precision
		q, err := resource.ParseQuantity(fmt.Sprintf("%v", metric.Value))
		if err != nil {
			log.Errorf("Could not parse the metric value: %v into the exponential format", metric.Value)
			continue
		}
		extMetric.value = external_metrics.ExternalMetricValue{
			MetricName:   metric.MetricName,
			MetricLabels: metric.Labels,
			Value:        q,
		}
		externalMetricsList = append(externalMetricsList, extMetric)

		externalMetricsInfoList = append(externalMetricsInfoList, provider.ExternalMetricInfo{
			Metric: metric.MetricName,
		})
	}
	p.externalMetrics = externalMetricsList
	p.timestamp = metav1.Now().Unix()
	log.Debugf("ListAllExternalMetrics returns %d metrics", len(externalMetricsInfoList))
	return externalMetricsInfoList
}

// GetExternalMetric is called every 30 seconds as a result of:
// - The registering of the External Metrics Provider
// - The creation of a HPA manifest with an External metrics type.
// - The validation of the metrics against Datadog
// Every replica answering to a ListAllExternalMetrics will populate its cache with a copy of the global cache.
// If the copy does not exist or is too old (>1 HPA controller default run cycle) we refresh it.
func (p *datadogProvider) GetExternalMetric(namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	matchingMetrics := []external_metrics.ExternalMetricValue{}

	p.ListAllExternalMetrics() // get up to date values from the cache or the Global Store

	for _, metric := range p.externalMetrics {
		metricFromDatadog := external_metrics.ExternalMetricValue{
			MetricName:   metric.info.Metric,
			MetricLabels: metric.value.MetricLabels,
			Value:        metric.value.Value,
		}
		// Datadog metrics are not case sensitive but the HPA Controller lower cases the metric name as it queries the metrics provider.
		// Lowering the metric name retrieved by the HPA Informer here, allows for users to use metrics with capital letters.
		// Datadog tags are lower cased, but metrics labels are not case sensitive.
		// If tags with capital letters are used (as the label selector in the HPA), no metrics will be retrieved from Datadog.
		if info.Metric == strings.ToLower(metric.info.Metric) &&
			metricSelector.Matches(labels.Set(metric.value.MetricLabels)) {
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

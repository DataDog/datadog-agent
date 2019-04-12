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

	"context"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"time"
)

type externalMetric struct {
	info  provider.ExternalMetricInfo
	value external_metrics.ExternalMetricValue
}

type datadogProvider struct {
	client dynamic.Interface
	mapper apimeta.RESTMapper

	externalMetrics []externalMetric
	resVersion      string
	store           Store
	serving         bool
	timestamp       int64
	maxAge          int64
}

// NewDatadogProvider creates a Custom Metrics and External Metrics Provider.
func NewDatadogProvider(ctx context.Context, client dynamic.Interface, mapper apimeta.RESTMapper, store Store) provider.MetricsProvider {
	maxAge := config.Datadog.GetInt64("external_metrics_provider.local_copy_refresh_rate")
	d := &datadogProvider{
		client: client,
		mapper: mapper,
		store:  store,
		maxAge: maxAge,
	}
	go d.externalMetricsSetter(ctx)
	return d
}

type timer struct {
	period time.Duration
	timer  time.Timer
}

func (t *timer) resetTimer() {
	t.timer.Stop()
	t.timer = *time.NewTimer(t.period)
}

func createTimer(period time.Duration) *timer {
	return &timer{period, *time.NewTimer(period)}
}

func (p *datadogProvider) externalMetricsSetter(ctx context.Context) {
	log.Infof("Starting async loop to collect External Metrics")
	tick := time.NewTicker(time.Duration(p.maxAge) * time.Second)
	defer tick.Stop()
	out := createTimer(2 * time.Duration(p.maxAge) * time.Second)
	defer out.timer.Stop()
	_, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-tick.C:
			var externalMetricsList []externalMetric

			rawMetrics, err := p.store.ListAllExternalMetricValues()
			if err != nil {
				log.Errorf("Could not list the external metrics in the store: %s", err.Error())
				p.serving = false
				break
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
			}
			p.externalMetrics = externalMetricsList
			p.timestamp = metav1.Now().Unix()
			p.serving = true

			out.resetTimer()

		case <-out.timer.C:
			p.serving = false
			log.Error("Timed out collecting External Metrics, stopping async loop")
			return
		}
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

// ListAllExternalMetrics lists the available External Metrics at the time.
func (p *datadogProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	if !p.serving {
		return nil
	}
	var externalMetricsInfoList []provider.ExternalMetricInfo
	log.Tracef("Listing available metrics as of %s", time.Unix(p.timestamp, 0).Format(time.RFC850))
	for _, metric := range p.externalMetrics {
		externalMetricsInfoList = append(externalMetricsInfoList, metric.info)
	}
	return externalMetricsInfoList
}

// GetExternalMetric is called by the PodAutoscaler Controller to get the value of the external metric it is currently evaluating.
func (p *datadogProvider) GetExternalMetric(namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {

	if time.Now().Unix()-p.timestamp > 2*p.maxAge || !p.serving {
		return nil, fmt.Errorf("external metrics invalid")
	}

	matchingMetrics := []external_metrics.ExternalMetricValue{}
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
	log.Debugf("External metrics returned: %#v", matchingMetrics)
	return &external_metrics.ExternalMetricValueList{
		Items: matchingMetrics,
	}, nil
}

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package externalmetrics

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/kubernetes-incubator/custom-metrics-apiserver/pkg/provider"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/metrics/pkg/apis/external_metrics"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	autogenExpirationPeriodHours int64 = 3
)

type datadogMetricProvider struct {
	apiCl            *apiserver.APIClient
	store            DatadogMetricsInternalStore
	autogenNamespace string
}

func NewDatadogMetricProvider(ctx context.Context, apiCl *apiserver.APIClient) (provider.ExternalMetricsProvider, error) {
	if apiCl == nil {
		return nil, fmt.Errorf("Impossible to create DatadogMetricProvider without valid APIClient")
	}

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as LeaderElection failed with: %v", err)
	}

	aggregator := config.Datadog.GetString("external_metrics.aggregator")
	rollup := config.Datadog.GetInt("external_metrics_provider.rollup")
	setQueryConfigValues(aggregator, rollup)

	refreshPeriod := config.Datadog.GetInt64("external_metrics_provider.refresh_period")
	retrieverMetricsMaxAge := int64(math.Max(config.Datadog.GetFloat64("external_metrics_provider.max_age"), float64(3*rollup)))
	autogenNamespace := common.GetResourcesNamespace()

	provider := &datadogMetricProvider{
		apiCl:            apiCl,
		store:            NewDatadogMetricsInternalStore(),
		autogenNamespace: autogenNamespace,
	}

	// Start MetricsRetriever, only leader will do refresh metrics
	dogCl, err := autoscalers.NewDatadogClient()
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as DatadogClient failed with: %v", err)
	}

	metricsRetriever, err := NewMetricsRetriever(refreshPeriod, retrieverMetricsMaxAge, autoscalers.NewProcessor(dogCl), le.IsLeader, &provider.store)
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as MetricsRetriever failed with: %v", err)
	}
	go metricsRetriever.Run(ctx.Done())

	// Start AutoscalerWatcher, only leader will flag DatadogMetrics as Active/Inactive
	// WPAInformerFactory is nil when WPA is not used. AutoscalerWatcher will check value itself.
	autoscalerWatcher, err := NewAutoscalerWatcher(refreshPeriod, autogenExpirationPeriodHours, autogenNamespace, apiCl.InformerFactory, apiCl.WPAInformerFactory, le.IsLeader, &provider.store)
	if err != nil {
		return nil, fmt.Errorf("Unabled to create DatadogMetricProvider as AutoscalerWatcher failed with: %v", err)
	}
	apiCl.InformerFactory.Start(ctx.Done())
	if apiCl.WPAInformerFactory != nil {
		apiCl.WPAInformerFactory.Start(ctx.Done())
	}
	go autoscalerWatcher.Run(ctx.Done())

	// We shift controller refresh period from retrieverRefreshPeriod to maximize the probability to have new data from DD
	controller, err := NewDatadogMetricController(apiCl.DDClient, apiCl.DDInformerFactory, le.IsLeader, &provider.store)
	if err != nil {
		return nil, fmt.Errorf("Unable to create DatadogMetricProvider as DatadogMetric Controller failed with: %v", err)
	}

	// Start informers & controllers (informers can be started multiple times)
	apiCl.DDInformerFactory.Start(ctx.Done())
	go controller.Run(ctx.Done())

	return provider, nil
}

func (p *datadogMetricProvider) GetExternalMetric(namespace string, metricSelector labels.Selector, info provider.ExternalMetricInfo) (*external_metrics.ExternalMetricValueList, error) {
	log.Debugf("Received external metric query with ns: %s, selector: %s, metricName: %s", namespace, metricSelector.String(), info.Metric)

	// Convert metric name to lower case to allow proper matching (and DD metrics are always lower case)
	info.Metric = strings.ToLower(info.Metric)

	// If the metric name is already prefixed, we can directly look up metrics in store
	datadogMetricID, parsed, hasPrefix := metricNameToDatadogMetricID(info.Metric)
	if !hasPrefix {
		datadogMetricID = p.autogenNamespace + kubernetesNamespaceSep + getAutogenDatadogMetricNameFromSelector(info.Metric, metricSelector)
		parsed = true
	}
	if !parsed {
		return nil, fmt.Errorf("ExternalMetric does not follow DatadogMetric format")
	}

	datadogMetric := p.store.Get(datadogMetricID)
	log.Debugf("DatadogMetric from store: %v", datadogMetric)

	if datadogMetric == nil {
		return nil, fmt.Errorf("DatadogMetric not found for metric name: %s, datadogmetricid: %s", info.Metric, datadogMetricID)
	}

	externalMetric, err := datadogMetric.ToExternalMetricFormat(info.Metric)
	if err != nil {
		return nil, err
	}

	return &external_metrics.ExternalMetricValueList{
		Items: []external_metrics.ExternalMetricValue{*externalMetric},
	}, nil
}

func (p *datadogMetricProvider) ListAllExternalMetrics() []provider.ExternalMetricInfo {
	datadogMetrics := p.store.GetAll()
	results := make([]provider.ExternalMetricInfo, 0, len(datadogMetrics))
	// Unique the external metric names
	autogenMetricNames := make(map[string]struct{})

	for _, datadogMetric := range datadogMetrics {
		if datadogMetric.Autogen {
			autogenMetricNames[datadogMetric.ExternalMetricName] = struct{}{}
		} else {
			results = append(results, provider.ExternalMetricInfo{Metric: datadogMetricIDToMetricName(datadogMetric.ID)})
		}
	}

	for metricName := range autogenMetricNames {
		results = append(results, provider.ExternalMetricInfo{Metric: metricName})
	}

	log.Tracef("Answering list of available metrics: %v", results)
	return results
}
